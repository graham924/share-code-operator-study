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
	workNum  = 5
	annoKey  = "ingress/http:true"
	maxRetry = 10
)

type controller struct {
	client        kubernetes.Interface
	serviceLister listercorev1.ServiceLister
	ingressLister listernetv1.IngressLister
	queue         workqueue.RateLimitingInterface
}

func NewController(clientset *kubernetes.Clientset, serviceInformer informercorev1.ServiceInformer, ingressInformer informernetv1.IngressInformer) *controller {
	c := controller{
		client:        clientset,
		serviceLister: serviceInformer.Lister(),
		ingressLister: ingressInformer.Lister(),
		queue:         workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "ingressManager"),
	}

	serviceInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    c.addService,
		UpdateFunc: c.updateService,
	})

	ingressInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		DeleteFunc: c.deleteIngress,
	})

	return &c
}

func (c *controller) enqueue(obj interface{}) {
	key, err := cache.MetaNamespaceKeyFunc(obj)
	if err != nil {
		runtime.HandleError(err)
		return
	}
	c.queue.Add(key)
}

func (c *controller) dequeue(item interface{}) {
	c.queue.Done(item)
}

func (c *controller) addService(obj interface{}) {
	c.enqueue(obj)
}

func (c *controller) updateService(oldObj interface{}, newObj interface{}) {
	if reflect.DeepEqual(oldObj, newObj) {
		return
	}
	// todo 比较annotation
	c.enqueue(newObj)
}

func (c *controller) deleteService(obj interface{}) {
	c.enqueue(obj)
}

func (c *controller) deleteIngress(obj interface{}) {
	ingress := obj.(*netv1.Ingress)
	ownerReference := metav1.GetControllerOf(ingress)
	if ownerReference == nil || ownerReference.Kind != "Service" {
		return
	}
	c.enqueue(obj)
}

func (c *controller) Run(stopCh chan struct{}) {
	for i := 0; i < workNum; i++ {
		go wait.Until(c.worker, time.Minute, stopCh)
	}
	<-stopCh
}

func (c *controller) worker() {
	for c.processNextItem() {

	}
}

func (c *controller) processNextItem() bool {
	item, shutdown := c.queue.Get()
	if shutdown {
		return false
	}
	defer c.dequeue(item)

	key := item.(string)
	err := c.syncService(key)
	if err != nil {
		c.handleError(key, err)
	}
	return true
}

func (c *controller) handleError(key string, err error) {
	if c.queue.NumRequeues(key) < maxRetry {
		c.queue.AddRateLimited(key)
		return
	}

	runtime.HandleError(err)
	c.queue.Forget(key)
}

func (c *controller) syncService(key string) error {
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

	// 检查service的annotation，是否包含 key: "ingress/http:true"
	_, ok := service.Annotations[annoKey]
	// 从indexer中，获取ingress
	ingress, err := c.ingressLister.Ingresses(namespace).Get(name)

	// service有"ingress/http:true"，但是ingress不存在，需要创建ingress
	if ok && errors.IsNotFound(err) {
		return nil
	}

	if !ok && ingress != nil {
		// ingress存在，但是service没有"ingress/http:true"，需要删除ingress
		err := c.client.NetworkingV1().Ingresses(namespace).Delete(context.TODO(), name, metav1.DeleteOptions{})
		if err != nil {
			return err
		}
	} else if ok && ingress == nil {
		// ingress不存在，但是service存在，需要创建ingress
		ig := c.createIngress(service)
		_, err := c.client.NetworkingV1().Ingresses(namespace).Create(context.TODO(), ig, metav1.CreateOptions{})
		if err != nil {
			return err
		}
	}

	return nil
}

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
