package main

import (
	"flag"
	"github.com/golang/glog"

	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	restclient "k8s.io/client-go/rest"
	//"k8s.io/client-go/tools/clientcmd"

	svclientset "github.com/cisco-sso/snapshot-validator/pkg/client/clientset/versioned"
	svs "github.com/cisco-sso/snapshot-validator/pkg/client/clientset/versioned/scheme"
	"github.com/cisco-sso/snapshot-validator/pkg/validator"
	snapshotclient "github.com/kubernetes-incubator/external-storage/snapshot/pkg/client"
	"k8s.io/sample-controller/pkg/signals"
)

func main() {
	stopCh := signals.SetupSignalHandler()
	flag.Parse()
	flag.Set("logtostderr", "true")

	clients := clients()
	controller := validator.NewController(clients, stopCh)

	if err := controller.Run(1, stopCh); err != nil {
		glog.Fatalf("Error running controller: %v", err)
	}
}

func buildConfig() (*restclient.Config, error) {
	/*kubeconfigPath := "/Users/jawoznia/tmp/kustomize/sre1.csco.cloud"
	masterUrl := "" // "64.102.180.113"
	return clientcmd.BuildConfigFromFlags(masterUrl, kubeconfigPath)*/
	return restclient.InClusterConfig()
}

func clients() validator.Clients {
	c := validator.Clients{}
	kubeconfig, err := buildConfig()
	if err != nil {
		glog.Fatalf("Error building kubeconfig: %v", err)
	}

	c.KubeClientset, err = kubernetes.NewForConfig(kubeconfig)
	if err != nil {
		glog.Fatalf("Error building kubernetes kube client: %v", err)
	}

	c.SnapshotClient, _, err = snapshotclient.NewClient(kubeconfig)
	if err != nil {
		glog.Fatalf("Error building kubernetes snapshot client: %v", err)
	}

	c.SvClientset, err = svclientset.NewForConfig(kubeconfig)
	if err != nil {
		glog.Fatalf("Error building kubernetes snapshot validator client: %v", err)
	}

	utilruntime.Must(svs.AddToScheme(scheme.Scheme))
	return c
}
