package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/nais/netroll/internal/netroller"
	"github.com/sirupsen/logrus"
	"k8s.io/client-go/kubernetes"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"

	// Load all client-go auth plugins
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
)

var (
	logLevel    string
	bindAddress string
)

func init() {
	flag.StringVar(&bindAddress, "bind-address", ":8080", "Bind address")
	flag.StringVar(&logLevel, "log-level", "debug", "Which log level to output")
}

func main() {
	flag.Parse()

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	defer cancel()

	log := newLogger()

	var kubeConfig *rest.Config
	var err error
	if envConfig := os.Getenv("KUBECONFIG"); envConfig != "" {
		kubeConfig, err = clientcmd.BuildConfigFromFlags("", envConfig)
		if err != nil {
			panic(err.Error())
		}
	} else {
		kubeConfig, err = rest.InClusterConfig()
		if err != nil {
			log.WithError(err).Fatal("failed to get kubeconfig")
		}
	}
	log.Debugf("starting netroller with kubeconfig: %s", kubeConfig.Host)

	k8sClient, err := kubernetes.NewForConfig(kubeConfig)
	if err != nil {
		log.WithError(err).Fatal("setting up k8s client")
	}

	dynamicClient, err := dynamic.NewForConfig(kubeConfig)
	if err != nil {
		log.WithError(err).Fatal("setting up dynamic client")
	}

	factory := dynamicinformer.NewFilteredDynamicSharedInformerFactory(dynamicClient, 0, corev1.NamespaceAll, nil)

	resource := factory.ForResource(schema.GroupVersionResource{
		Group:    "sql.cnrm.cloud.google.com",
		Version:  "v1beta1",
		Resource: "sqlinstances",
	})

	informer := resource.Informer()
	informer.SetWatchErrorHandler(errorHandler)

	nr := netroller.New(log, k8sClient)

	informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    nr.Add,
		UpdateFunc: nr.Update,
	})

	go informer.Run(ctx.Done())
	waitForCacheSync(ctx.Done(), informer.HasSynced)

	ticker := time.NewTicker(15 * time.Second)
	for {
		select {
		case <-ticker.C:
		case <-ctx.Done():
			return
		}
	}
}

func errorHandler(r *cache.Reflector, err error) {
	fmt.Println("watch error ", err)
}

func newLogger() *logrus.Logger {
	log := logrus.StandardLogger()
	log.SetFormatter(&logrus.JSONFormatter{})

	l, err := logrus.ParseLevel(logLevel)
	if err != nil {
		log.Fatal(err)
	}
	log.SetLevel(l)
	return log
}

func waitForCacheSync(stop <-chan struct{}, cacheSyncs ...cache.InformerSynced) bool {
	max := time.Millisecond * 100
	delay := time.Millisecond
	f := func() bool {
		for _, syncFunc := range cacheSyncs {
			if !syncFunc() {
				return false
			}
		}
		return true
	}
	for {
		select {
		case <-stop:
			return false
		default:
		}
		res := f()
		if res {
			return true
		}
		delay *= 2
		if delay > max {
			delay = max
		}

		select {
		case <-stop:
			return false
		case <-time.After(delay):
		}
	}
}
