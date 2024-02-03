package main

import (
	"crd-controller-demo/pkg/controller"
	clientset "crd-controller-demo/pkg/generated/clientset/versioned"
	appinformers "crd-controller-demo/pkg/generated/informers/externalversions"
	"crd-controller-demo/pkg/signals"
	"crd-controller-demo/pkg/utils"
	"flag"
	kubeinformers "k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"time"

	"k8s.io/klog/v2"
)

var (
	masterURL  string
	kubeConfig string
)

func main() {
	klog.InitFlags(nil)
	flag.Parse()

	// set up signals so we handle the first shutdown signal gracefully
	stopCh := signals.SetupSignalHandler()

	config, err := clientcmd.BuildConfigFromFlags(masterURL, kubeConfig)
	//config, err := clientcmd.BuildConfigFromFlags("", clientcmd.RecommendedHomeFile)
	if err != nil {
		klog.Fatalf("Error building kubeConfig: %s", err.Error())
	}

	kubeClientSet, err := kubernetes.NewForConfig(config)
	if err != nil {
		klog.Fatalf("Error building kubernetes clientset: %s", err.Error())
	}

	appClientSet, err := clientset.NewForConfig(config)
	if err != nil {
		klog.Fatalf("Error building app clientset: %s", err.Error())
	}

	kubeInformerFactory := kubeinformers.NewSharedInformerFactory(kubeClientSet, time.Second*30)
	appInformerFactory := appinformers.NewSharedInformerFactory(appClientSet, time.Second*30)

	controller := controller.NewController(kubeClientSet, appClientSet,
		kubeInformerFactory.Apps().V1().Deployments(),
		kubeInformerFactory.Core().V1().Services(),
		appInformerFactory.Appcontroller().V1().Apps())

	// notice that there is no need to run Start methods in a separate goroutine. (i.e. go kubeInformerFactory.Start(stopCh)
	// Start method is non-blocking and runs all registered informers in a dedicated goroutine.
	kubeInformerFactory.Start(stopCh)
	appInformerFactory.Start(stopCh)

	if err = controller.Run(utils.WorkNum, stopCh); err != nil {
		klog.Fatalf("Error running controller: %s", err.Error())
	}
}

func init() {
	flag.StringVar(&kubeConfig, "kubeConfig", "", "Path to a kubeConfig. Only required if out-of-cluster.")
	flag.StringVar(&masterURL, "master", "", "The address of the Kubernetes API server. Overrides any value in kubeConfig. Only required if out-of-cluster.")
}
