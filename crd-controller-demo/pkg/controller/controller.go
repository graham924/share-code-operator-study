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
	// 从 workqueue 中获取一个item
	item, shutdown := c.workqueue.Get()
	// 如果队列已经被回收，返回false
	if shutdown {
		return false
	}

	// 最终将这个item标记为已处理
	defer c.workqueue.Done(item)

	// 将item转成key
	key, ok := item.(string)
	if !ok {
		klog.Warningf("failed convert item [%s] to string", item)
		c.workqueue.Forget(item)
		return true
	}

	// 对key这个App，进行具体的调谐。这里面是核心的调谐逻辑
	if err := c.syncApp(key); err != nil {
		klog.Errorf("failed to syncApp [%s], error: [%s]", key, err.Error())
		c.handleError(key, err)
	}

	return true
}

// syncApp 对App资源的调谐核心逻辑
func (c *Controller) syncApp(key string) error {
	// 将key拆分成namespace、name
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return err
	}

	// 从informer缓存中，获取到key对应的app对象
	app, err := c.appsLister.Apps(namespace).Get(name)
	if err != nil {
		if errors.IsNotFound(err) {
			return fmt.Errorf("app [%s] in work queue no longer exists", key)
		}
		return err
	}

	// 取出 app 对象 的 deploymentSpec 部分
	deploymentTemplate := app.Spec.DeploymentSpec
	// 如果 app 的 deploymentTemplate 不为空
	if deploymentTemplate.Name != "" {
		// 尝试从缓存获取 对应的 deployment
		deploy, err := c.deploymentsLister.Deployments(namespace).Get(deploymentTemplate.Name)
		if err != nil {
			// 如果没找到
			if errors.IsNotFound(err) {
				klog.V(4).Info("starting to create deployment [%s] in namespace [%s]", deploymentTemplate.Name, namespace)
				// 创建一个deployment对象，然后使用 kubeClientset，与apiserver交互，创建deployment
				deploy = newDeployment(deploymentTemplate, app)
				_, err := c.kubeClientset.AppsV1().Deployments(namespace).Create(context.TODO(), deploy, metav1.CreateOptions{})
				if err != nil {
					return fmt.Errorf("failed to create deployment [%s] in namespace [%s], error: [%v]", deploymentTemplate.Name, namespace, err)
				}
				// 创建完成后，从apiserver中，获取最新的deployment，因为下面要使用它的status.【这里不能从informer缓存获取，因为缓存里暂时未同步新创建的deployment】
				deploy, _ = c.kubeClientset.AppsV1().Deployments(namespace).Get(context.TODO(), deploymentTemplate.Name, metav1.GetOptions{})
			} else {
				return fmt.Errorf("failed to get deployment [%s] in namespace [%s], error: [%v]", deploy.Name, deploy.Namespace, err)
			}
		}
		// 如果获取到的 deployment，并非 app 所控制，报错
		if !metav1.IsControlledBy(deploy, app) {
			msg := fmt.Sprintf(utils.MessageResourceExists, deploy.Name)
			c.recorder.Event(app, corev1.EventTypeWarning, utils.ErrResourceExists, msg)
			return fmt.Errorf("%s", msg)
		}
		// update deploy status
		app.Status.DeploymentStatus = &deploy.Status
	}

	// 取出 app 对象 的 deploymentSpec 部分
	serviceTemplate := app.Spec.ServiceSpec
	// 如果 app 的 serviceTemplate 不为空
	if serviceTemplate.Name != "" {
		// 尝试从缓存获取 对应的 service
		service, err := c.servicesLister.Services(namespace).Get(serviceTemplate.Name)
		if err != nil {
			// 如果没找到
			if errors.IsNotFound(err) {
				klog.V(4).Info("starting to create service [%s] in namespace [%s]", serviceTemplate.Name, namespace)
				// 创建一个service对象，然后使用 kubeClientset，与apiserver交互，创建service
				service = newService(serviceTemplate, app)
				_, err := c.kubeClientset.CoreV1().Services(namespace).Create(context.TODO(), service, metav1.CreateOptions{})
				if err != nil {
					return fmt.Errorf("failed to create service [%s] in namespace [%s], error: [%v]", serviceTemplate.Name, namespace, err)
				}
				// 创建完成后，从apiserver中，获取最新的service，因为下面要使用它的status.【这里不能从informer缓存获取，因为缓存里暂时未同步新创建的service】
				service, _ = c.kubeClientset.CoreV1().Services(namespace).Get(context.TODO(), serviceTemplate.Name, metav1.GetOptions{})
			} else {
				return fmt.Errorf("failed to get service [%s] in namespace [%s], error: [%v]", service.Name, service.Namespace, err)
			}
		}
		// 如果获取到的 service，并非 app 所控制，报错
		if !metav1.IsControlledBy(service, app) {
			msg := fmt.Sprintf(utils.MessageResourceExists, service.Name)
			c.recorder.Event(app, corev1.EventTypeWarning, utils.ErrResourceExists, msg)
			return fmt.Errorf("%s", msg)
		}
		// update service status
		app.Status.ServiceStatus = &service.Status
	}

	// 处理完 deploymentSpec、serviceSpec，将设置好的AppStatus更新到环境中去
	_, err = c.appClientset.AppcontrollerV1().Apps(namespace).Update(context.TODO(), app, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to update app [%s], error: [%v]", key, err)
	}

	// 记录事件日志
	c.recorder.Event(app, corev1.EventTypeNormal, utils.SuccessSynced, utils.MessageResourceSynced)

	return nil
}

// newDeployment 创建一个deployment对象
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
			// Selector 和 pod 的 Labels 必须一致
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app-key": "app-value",
				},
			},
			Replicas: &template.Replicas,
			Template: corev1.PodTemplateSpec{
				// pod 的 labels，没有让用户指定，这里设置成默认的
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app-key": "app-value",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "app-deploy-container",
							Image: template.Image,
						},
					},
				},
			},
		},
	}
	// 将 deploy 的 OwnerReferences，设置成app
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
		Spec: corev1.ServiceSpec{
			// Selector 和 pod 的 Labels 必须一致
			Selector: map[string]string{
				"app-key": "app-value",
			},
			Ports: []corev1.ServicePort{
				{
					Name: "app-service",
					// Service的端口，默认设置成了8080。这里仅仅是为了学习crd，实际开发中可以设置到AppSpec中去
					Port: 8080,
				},
			},
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
