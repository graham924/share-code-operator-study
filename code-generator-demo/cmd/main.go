package main

import (
	"code-generator-demo/pkg/generated/clientset/versioned/typed/appcontroller/v1alpha1"
	"fmt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/clientcmd"
)

func main() {
	config, err := clientcmd.BuildConfigFromFlags("", clientcmd.RecommendedHomeFile)
	if err != nil {
		panic(err.Error())
	}

	appClient, err := v1alpha1.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}

	appList, err := appClient.Applications("tcs").List(metav1.ListOptions{})
	if err != nil {
		panic(err.Error())
	}

	for _, app := range appList.Items {
		fmt.Println(app.Name)
	}
}
