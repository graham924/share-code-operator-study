package controller

import (
	"context"
	appcontrollerv1 "crd-controller-demo/pkg/apis/appcontroller/v1"
	clientset "crd-controller-demo/pkg/generated/clientset/versioned"
	"crd-controller-demo/pkg/generated/clientset/versioned/scheme"
	informersv1 "crd-controller-demo/pkg/generated/informers/externalversions/appcontroller/v1"
	listerv1 "crd-controller-demo/pkg/generated/listers/appcontroller/v1"
	"crd-controller-demo/pkg/utils"
	"fmt"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	appsinformersv1 "k8s.io/client-go/informers/apps/v1"
	coreinformersv1 "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	appslisterv1 "k8s.io/client-go/listers/apps/v1"
	corelisterv1 "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"
	"reflect"
	"time"
)

type Controller struct {
	// kubeClientset kubernetes 所有内置资源的 clientset，用于操作所有内置资源
	kubeClientset kubernetes.Interface
	// appClientset 为 apps 资源生成的 clientset，用于操作 apps 资源
	appClientset clientset.Interface

	// deploymentsLister 查询本地缓存中的 deployment 资源
	deploymentsLister appslisterv1.DeploymentLister
	// servicesLister 查询本地缓存中的 service 资源
	servicesLister corelisterv1.ServiceLister
	// appsLister 查询本地缓存中的 apps 资源
	appsLister listerv1.AppLister

	// deploymentsSync 检查 deployments 资源，是否完成同步
	deploymentsSync cache.InformerSynced
	// servicesSync 检查 services 资源，是否完成同步
	servicesSync cache.InformerSynced
	// appsSync 检查 apps 资源，是否完成同步
	appsSync cache.InformerSynced

	// workqueue 队列，存储 待处理资源的key（一般是 namespace/name）
	workqueue workqueue.RateLimitingInterface
	// recorder 事件记录器
	recorder record.EventRecorder
}

func NewController(kubeclientset kubernetes.Interface,
	appclientset clientset.Interface,
	deploymentInformer appsinformersv1.DeploymentInformer,
	serviceInformer coreinformersv1.ServiceInformer,
	appInformer informersv1.AppInformer) *Controller {

	// 将 为apps资源生成的clientset的Scheme，添加到全局 Scheme 中
	utilruntime.Must(scheme.AddToScheme(scheme.Scheme))

	klog.V(4).Info("Creating event broadcaster")
	// 新建一个事件广播器，用于将事件广播到不同的监听器
	eventBroadcaster := record.NewBroadcaster()
	// 将事件以结构化日志的形式输出
	eventBroadcaster.StartStructuredLogging(0)
	// 将事件广播器配置为将事件记录到指定的 EventSink
	eventBroadcaster.StartRecordingToSink(&typedcorev1.EventSinkImpl{Interface: kubeclientset.CoreV1().Events("")})
	// 创建一个事件记录器，用于发送事件到设置好的事件广播
	recorder := eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: utils.ControllerAgentName})

	// 创建一个 Controller 对象
	c := &Controller{
		kubeClientset:     kubeclientset,
		appClientset:      appclientset,
		deploymentsLister: deploymentInformer.Lister(),
		servicesLister:    serviceInformer.Lister(),
		appsLister:        appInformer.Lister(),
		deploymentsSync:   deploymentInformer.Informer().HasSynced,
		servicesSync:      serviceInformer.Informer().HasSynced,
		appsSync:          appInformer.Informer().HasSynced,
		workqueue:         workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "Apps"),
		recorder:          recorder,
	}

	// 为AppInformer，设置 ResourceEventHandler
	klog.Info("Setting up event handlers")
	appInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    c.AddApp,
		UpdateFunc: c.UpdateApp,
	})

	// 为 DeploymentInformer，设置 ResourceEventHandler
	deploymentInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		DeleteFunc: c.DeleteDeployment,
	})

	// 为 ServiceInformer，设置 ResourceEventHandler
	serviceInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		DeleteFunc: c.DeleteService,
	})

	// 将控制器实例返回
	return c
}

func (c *Controller) enqueue(obj interface{}) {
	key, err := cache.MetaNamespaceKeyFunc(obj)
	if err != nil {
		utilruntime.HandleError(err)
		return
	}
	c.workqueue.Add(key)
}

func (c *Controller) AddApp(obj interface{}) {
	c.enqueue(obj)
}

func (c *Controller) UpdateApp(oldObj, newObj interface{}) {
	if reflect.DeepEqual(oldObj, newObj) {
		key, _ := cache.MetaNamespaceKeyFunc(oldObj)
		klog.V(4).Infof("UpdateApp %s: %s", key, "no change")
		return
	}
	c.enqueue(newObj)
}

func (c *Controller) DeleteDeployment(obj interface{}) {
	deploy := obj.(*appsv1.Deployment)
	ownerReference := metav1.GetControllerOf(deploy)
	if ownerReference == nil || ownerReference.Kind != "App" {
		return
	}
	c.enqueue(obj)
}

func (c *Controller) DeleteService(obj interface{}) {
	service := obj.(*corev1.Service)
	ownerReference := metav1.GetControllerOf(service)
	if ownerReference == nil || ownerReference.Kind != "App" {
		return
	}
	c.enqueue(obj)
}

func (c *Controller) Run(workerNum int, stopCh <-chan struct{}) error {
	// 用于处理程序崩溃，发生未捕获的异常（panic）时，调用HandleCrash()方法，记录日志并发出报告
	defer utilruntime.HandleCrash()
	// 控制器程序结束时，清理队列
	defer c.workqueue.ShutDown()

	klog.V(4).Info("Starting App Controller")

	klog.V(4).Info("Waiting for informer cache to sync")
	if ok := cache.WaitForCacheSync(stopCh, c.appsSync, c.deploymentsSync, c.servicesSync); !ok {
		return fmt.Errorf("failed to wait for caches to sync")
	}

	klog.V(4).Info("Starting workers")
	for i := 0; i < workerNum; i++ {
		go wait.Until(c.worker, time.Minute, stopCh)
	}

	klog.V(4).Info("Started workers")
	<-stopCh
	klog.V(4).Info("Shutting down workers")

	return nil
}

func (c *Controller) worker() {
	for c.processNextWorkItem() {
	}
}

func (c *Controller) processNextWorkItem() bool {
	item, shutdown := c.workqueue.Get()
	if shutdown {
		return false
	}

	defer c.workqueue.Done(item)

	key, ok := item.(string)
	if !ok {
		klog.Warningf("failed convert item [%s] to string", item)
		c.workqueue.Forget(item)
		return true
	}

	if err := c.syncApp(key); err != nil {
		klog.Errorf("failed to syncApp [%s], error: [%s]", key, err.Error())
		c.handleError(key, err)
	}

	return true

}

func (c *Controller) syncApp(key string) error {
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return err
	}

	app, err := c.appsLister.Apps(namespace).Get(name)
	if err != nil {
		if errors.IsNotFound(err) {
			return fmt.Errorf("app [%s] in work queue no longer exists", key)
		}
		return err
	}
	deploymentTemplate := app.Spec.DeploymentSpec
	if deploymentTemplate.Name != "" {
		deploy, err := c.deploymentsLister.Deployments(namespace).Get(deploymentTemplate.Name)
		if err != nil {
			if errors.IsNotFound(err) {
				klog.V(4).Info("starting to create deployment [%s] in namespace [%s]", deploy.Name, deploy.Namespace)
				deploy = newDeployment(deploymentTemplate, app)
				_, err := c.kubeClientset.AppsV1().Deployments(namespace).Create(context.TODO(), deploy, metav1.CreateOptions{})
				if err != nil {
					return fmt.Errorf("failed to create deployment [%s] in namespace [%s], error: [%v]", deploy.Name, deploy.Namespace, err)
				}
				deploy, _ = c.deploymentsLister.Deployments(namespace).Get(deploymentTemplate.Name)
			} else {
				return fmt.Errorf("failed to get deployment [%s] in namespace [%s], error: [%v]", deploy.Name, deploy.Namespace, err)
			}
		}
		if !metav1.IsControlledBy(deploy, app) {
			msg := fmt.Sprintf(utils.MessageResourceExists, deploy.Name)
			c.recorder.Event(app, corev1.EventTypeWarning, utils.ErrResourceExists, msg)
			return fmt.Errorf("%s", msg)
		}
		// update deploy status
		app.Status.DeploymentStatus = &deploy.Status
	}

	serviceTemplate := app.Spec.ServiceSpec
	if serviceTemplate.Name != "" {
		service, err := c.servicesLister.Services(namespace).Get(serviceTemplate.Name)
		if err != nil {
			if errors.IsNotFound(err) {
				klog.V(4).Info("starting to create deployment [%s] in namespace [%s]", service.Name, service.Namespace)
				service = newService(serviceTemplate, app)
				_, err := c.kubeClientset.CoreV1().Services(namespace).Create(context.TODO(), service, metav1.CreateOptions{})
				if err != nil {
					return fmt.Errorf("failed to create deployment [%s] in namespace [%s], error: [%v]", service.Name, service.Namespace, err)
				}
				service, _ = c.servicesLister.Services(namespace).Get(serviceTemplate.Name)
			} else {
				return fmt.Errorf("failed to get deployment [%s] in namespace [%s], error: [%v]", service.Name, service.Namespace, err)
			}
		}
		if !metav1.IsControlledBy(service, app) {
			msg := fmt.Sprintf(utils.MessageResourceExists, service.Name)
			c.recorder.Event(app, corev1.EventTypeWarning, utils.ErrResourceExists, msg)
			return fmt.Errorf("%s", msg)
		}
		// update service status
		app.Status.ServiceStatus = &service.Status
	}

	_, err = c.appClientset.AppcontrollerV1().Apps(namespace).Update(context.TODO(), app, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to update app [%s], error: [%v]", key, err)
	}

	c.recorder.Event(app, corev1.EventTypeNormal, utils.SuccessSynced, utils.MessageResourceSynced)

	return nil
}

func newDeployment(template appcontrollerv1.DeploymentTemplate, app *appcontrollerv1.App) *appsv1.Deployment {
	d := &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Deployment",
			APIVersion: "apps/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: template.Name,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &template.Replicas,
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Image: template.Image,
						},
					},
				},
			},
		},
	}
	d.OwnerReferences = []metav1.OwnerReference{
		*metav1.NewControllerRef(app, appcontrollerv1.SchemeGroupVersion.WithKind("App")),
	}
	return d
}

func newService(template appcontrollerv1.ServiceTemplate, app *appcontrollerv1.App) *corev1.Service {
	s := &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Service",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: template.Name,
		},
	}

	s.OwnerReferences = []metav1.OwnerReference{
		*metav1.NewControllerRef(app, appcontrollerv1.SchemeGroupVersion.WithKind("App")),
	}
	return s
}

func (c *Controller) handleError(key string, err error) {
	// 如果当前key的处理次数，还不到最大重试次数，则再次加入队列
	if c.workqueue.NumRequeues(key) < utils.MaxRetry {
		c.workqueue.AddRateLimited(key)
		return
	}

	// 运行时统一处理错误
	utilruntime.HandleError(err)
	// 不再处理这个key
	c.workqueue.Forget(key)
}
