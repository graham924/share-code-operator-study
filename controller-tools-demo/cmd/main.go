package main

import (
	"context"
	"controller-tools-demo/pkg/apis/appcontroller/v1alpha1"
	"fmt"
	"log"

	"k8s.io/client-go/kubernetes/scheme"

	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

func main() {
	config, err := clientcmd.BuildConfigFromFlags("", clientcmd.RecommendedHomeFile)
	if err != nil {
		log.Fatalln(err)
	}

	config.APIPath = "/apis/"
	config.GroupVersion = &v1alpha1.SchemeGroupVersion
	config.NegotiatedSerializer = scheme.Codecs

	client, err := rest.RESTClientFor(config)
	if err != nil {
		log.Fatalln(err)
	}

	app := v1alpha1.Application{}
	err = client.Get().Namespace("tcs").Resource("applications").Name("testapp").Do(context.TODO()).Into(&app)
	if err != nil {
		log.Fatalln(err)
	}

	newObj := app.DeepCopy()
	newObj.Spec.Name = "testapp2"

	fmt.Println(app.Spec.Name)
	fmt.Println(app.Spec.Replicas)

	fmt.Println(newObj.Spec.Name)
}
