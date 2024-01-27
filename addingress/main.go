package main

import (
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"log"
	"share-code-operator-study/addingress/pkg"
)

func main() {
	// 创建一个 集群客户端配置
	config, err := clientcmd.BuildConfigFromFlags("", clientcmd.RecommendedHomeFile)
	if err != nil {
		inClusterConfig, err := rest.InClusterConfig()
		if err != nil {
			log.Fatalln("can't get config")
		}
		config = inClusterConfig
	}

	// 创建一个 clientset 客户端，用于创建 informerFactory
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}

	// 创建一个 informerFactory
	factory := informers.NewSharedInformerFactory(clientset, 0)
	// 使用 informerFactory 创建Services资源的 informer对象
	serviceInformer := factory.Core().V1().Services()
	// 使用 informerFactory 创建Ingresses资源的 informer对象
	ingressInformer := factory.Networking().V1().Ingresses()

	// 创建一个自定义控制器
	controller := pkg.NewController(clientset, serviceInformer, ingressInformer)

	// 创建 停止channel信号
	stopCh := make(chan struct{})
	// 启动 informerFactory，会启动已经创建的 serviceInformer、ingressInformer
	factory.Start(stopCh)
	// 等待 所有informer 从 etcd 实现全量同步
	factory.WaitForCacheSync(stopCh)

	// 启动自定义控制器
	controller.Run(stopCh)
}
