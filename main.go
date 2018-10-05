package main

import (
	"flag"
	"github.com/golang/glog"

	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	restclient "k8s.io/client-go/rest"

	svclientset "github.com/cisco-sso/snapshot-validator/pkg/client/clientset/versioned"
	svs "github.com/cisco-sso/snapshot-validator/pkg/client/clientset/versioned/scheme"
	snapshotclient "github.com/kubernetes-incubator/external-storage/snapshot/pkg/client"
	"k8s.io/sample-controller/pkg/signals"
)

func main() {
	stopCh := signals.SetupSignalHandler()
	flag.Parse()
	flag.Set("logtostderr", "true")

	kubeClient, snapshotClient, svClient := clients()
	controller := NewController(kubeClient, snapshotClient, svClient, stopCh)

	if err := controller.Run(2, stopCh); err != nil {
		glog.Fatalf("Error running controller: %v", err)
	}
}

func clients() (kubernetes.Interface, *restclient.RESTClient, svclientset.Interface) {
	kubeconfig, err := restclient.InClusterConfig()
	if err != nil {
		glog.Fatalf("Error building kubeconfig: %v", err)
	}

	kubeClient, err := kubernetes.NewForConfig(kubeconfig)
	if err != nil {
		glog.Fatalf("Error building kubernetes kube client: %v", err)
	}

	snapshotClient, _, err := snapshotclient.NewClient(kubeconfig)
	if err != nil {
		glog.Fatalf("Error building kubernetes snapshot client: %v", err)
	}

	svClient, err := svclientset.NewForConfig(kubeconfig)
	if err != nil {
		glog.Fatalf("Error building kubernetes snapshot validator client: %v", err)
	}

	utilruntime.Must(svs.AddToScheme(scheme.Scheme))
	return kubeClient, snapshotClient, svClient
}
