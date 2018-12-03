package main

import (
	"flag"
	"github.com/golang/glog"

	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/clientcmd"

	svclientset "github.com/cisco-sso/snapshot-manager/pkg/client/clientset/versioned"
	svs "github.com/cisco-sso/snapshot-manager/pkg/client/clientset/versioned/scheme"
	"github.com/cisco-sso/snapshot-manager/pkg/validator"
	snapshotclient "github.com/kubernetes-incubator/external-storage/snapshot/pkg/client"
	"k8s.io/sample-controller/pkg/signals"
)

var (
	masterURL  string
	kubeconfig string
)

func init() {
	flag.StringVar(&kubeconfig, "kubeconfig", "", "Path to a kubeconfig. Only required if out-of-cluster.")
	flag.StringVar(&masterURL, "master", "", "The address of the Kubernetes API server. Overrides any value in kubeconfig. Only required if out-of-cluster.")
}

func main() {
	stopCh := signals.SetupSignalHandler()
	flag.Parse()
	flag.Set("logtostderr", "true")

	clients := clients()
	controller := validator.NewController(clients, stopCh)

	if err := controller.Run(2, stopCh); err != nil {
		glog.Fatalf("Error running controller: %v", err)
	}
}

func clients() validator.Clients {
	c := validator.Clients{}
	kubeconfig, err := clientcmd.BuildConfigFromFlags(masterURL, kubeconfig)
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
