package pkg

import (
	"context"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	informercorev1 "k8s.io/client-go/informers/core/v1"
	informernetv1 "k8s.io/client-go/informers/networking/v1"
	"k8s.io/client-go/kubernetes"
	listercorev1 "k8s.io/client-go/listers/core/v1"
	listernetv1 "k8s.io/client-go/listers/networking/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"reflect"
	"time"
)

const (
	// worker 数量
	workNum = 5
	// service 指定 ingress 的 annotation key
	annoKey = "ingress/http"
	// 调谐失败的最大重试次数
	maxRetry = 10
)

// 自定义控制器
type controller struct {
	client        kubernetes.Interface
	serviceLister listercorev1.ServiceLister
	ingressLister listernetv1.IngressLister
	queue         workqueue.RateLimitingInterface
}

// NewController 创建一个自定义控制器
func NewController(clientset *kubernetes.Clientset, serviceInformer informercorev1.ServiceInformer, ingressInformer informernetv1.IngressInformer) *controller {
	// 控制器中，包含一个clientset、service和ingress的缓存监听器、一个workqueue
	c := controller{
		client:        clientset,
		serviceLister: serviceInformer.Lister(),
		ingressLister: ingressInformer.Lister(),
		queue:         workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "ingressManager"),
	}

	// 为 serviceInformer 添加 ResourceEventHandler
	serviceInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		// 添加service时触发
		AddFunc: c.addService,
		// 修改service时触发
		UpdateFunc: c.updateService,
		// 这里没有删除service的逻辑，因为我们会使用 OwnerReferences 将service+ingress关联起来。
		// 因此删除service，会由kubernetes的ControllerManager中的特殊Controller，自动完成ingress的gc
	})

	// 为 ingressInformer 添加 ResourceEventHandler
	ingressInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		// 删除ingress时触发
		DeleteFunc: c.deleteIngress,
	})

	return &c
}

// 添加service时触发
func (c *controller) addService(obj interface{}) {
	// 将 添加service 的 key 加入 workqueue
	c.enqueue(obj)
}

// 修改service时触发
func (c *controller) updateService(oldObj interface{}, newObj interface{}) {
	// 如果两个对象一致，就无需触发修改逻辑
	if reflect.DeepEqual(oldObj, newObj) {
		return
	}
	// todo 比较annotation
	// 将 修改service 的 key 加入 workqueue
	c.enqueue(newObj)
}

// 删除ingress时触发
func (c *controller) deleteIngress(obj interface{}) {
	// 将对象转成ingress，并获取到它的 ownerReference
	ingress := obj.(*netv1.Ingress)
	ownerReference := metav1.GetControllerOf(ingress)
	// 如果ingress的 ownerReference 没有绑定到service，则无需处理
	if ownerReference == nil || ownerReference.Kind != "Service" {
		return
	}
	// 如果ingress的 ownerReference 已经绑定到service，则需要处理
	c.enqueue(obj)
}

// enqueue 将 待添加service 的 key 加入 workqueue
func (c *controller) enqueue(obj interface{}) {
	// 调用工具方法，获取 kubernetes资源对象的 key（默认是 ns/name，或 name）
	key, err := cache.MetaNamespaceKeyFunc(obj)
	// 获取失败，不加入队列，即本次事件不予处理
	if err != nil {
		runtime.HandleError(err)
		return
	}
	// 将 key 加入 workqueue
	c.queue.Add(key)
}

// dequeue 将处理完成的 key 出队
func (c *controller) dequeue(item interface{}) {
	c.queue.Done(item)
}

// Run 启动controller
func (c *controller) Run(stopCh chan struct{}) {
	// 启动多个worker，同时对workqueue中的事件进行处理，效率提升5倍
	for i := 0; i < workNum; i++ {
		// 每个worker都是一个协程，使用同一个停止信号
		go wait.Until(c.worker, time.Minute, stopCh)
	}
	// 启动完成后，Run函数就停止在这里，等待停止信号
	<-stopCh
}

// worker方法
func (c *controller) worker() {
	// 死循环，worker处理完一个，再去处理下一个
	for c.processNextItem() {

	}
}

// processNextItem 处理下一个
func (c *controller) processNextItem() bool {
	// 从 workerqueue 取出一个key
	item, shutdown := c.queue.Get()
	// 如果已经收到停止信号了，则返回false，worker就会停止处理
	if shutdown {
		return false
	}
	// 处理完成后，将这个key出队
	defer c.dequeue(item)

	// 转成string类型的key
	key := item.(string)

	// 处理service逻辑的核心方法
	err := c.syncService(key)
	// 处理过程出错，进入错误统一处理逻辑
	if err != nil {
		c.handleError(key, err)
	}
	// 处理结束，返回true
	return true
}

// handleError 错误统一处理逻辑
func (c *controller) handleError(key string, err error) {
	// 如果当前key的处理次数，还不到最大重试次数，则再次加入队列
	if c.queue.NumRequeues(key) < maxRetry {
		c.queue.AddRateLimited(key)
		return
	}

	// 运行时统一处理错误
	runtime.HandleError(err)
	// 不再处理这个key
	c.queue.Forget(key)
}

// syncService 处理service逻辑的核心方法
func (c *controller) syncService(key string) error {
	// 将 key 切割为 ns 和 name
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return err
	}

	// 从indexer中，获取service
	service, err := c.serviceLister.Services(namespace).Get(name)
	// 没有service，直接返回
	if errors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}

	// 检查service的annotation，是否包含 key: "ingress/http"
	_, ok := service.Annotations[annoKey]
	// 从indexer缓存中，获取ingress
	ingress, err := c.ingressLister.Ingresses(namespace).Get(name)

	if ok && errors.IsNotFound(err) {
		// ingress不存在，但是service有"ingress/http"，需要创建ingress
		// 创建ingress
		ig := c.createIngress(service)
		// 调用controller中的client，完成ingress的创建
		_, err := c.client.NetworkingV1().Ingresses(namespace).Create(context.TODO(), ig, metav1.CreateOptions{})
		if err != nil {
			return err
		}
	} else if !ok && ingress != nil {
		// ingress存在，但是service没有"ingress/http"，需要删除ingress
		// 调用controller中的client，完成ingress的删除
		err := c.client.NetworkingV1().Ingresses(namespace).Delete(context.TODO(), name, metav1.DeleteOptions{})
		if err != nil {
			return err
		}
	}

	return nil
}

// createIngress 创建ingress
func (c *controller) createIngress(service *corev1.Service) *netv1.Ingress {
	icn := "ingress"
	pathType := netv1.PathTypePrefix
	return &netv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      service.Name,
			Namespace: service.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(service, corev1.SchemeGroupVersion.WithKind("Service")),
			},
		},
		Spec: netv1.IngressSpec{
			IngressClassName: &icn,
			Rules: []netv1.IngressRule{
				{
					Host: "example.com",
					IngressRuleValue: netv1.IngressRuleValue{
						HTTP: &netv1.HTTPIngressRuleValue{
							Paths: []netv1.HTTPIngressPath{
								{
									Path:     "/",
									PathType: &pathType,
									Backend: netv1.IngressBackend{
										Service: &netv1.IngressServiceBackend{
											Name: service.Name,
											Port: netv1.ServiceBackendPort{
												Number: 80,
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
}
