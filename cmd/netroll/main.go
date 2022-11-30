package main

import (
	"context"
	"flag"
	"fmt"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	"os"
	"path/filepath"
	"time"

	// Load all client-go auth plugins
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"github.com/sirupsen/logrus"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/util/homedir"
)

type Config struct {
	BindAddress  string
	LogLevel     string
	GCPProjectID string
	PubsubTopic  string
	Production   bool
	Env          string
}

var cfg = DefaultConfig()

func init() {
	flag.StringVar(&cfg.BindAddress, "bind-address", cfg.BindAddress, "Bind address")
	flag.StringVar(&cfg.LogLevel, "log-level", "info", "which log level to output")
	flag.StringVar(&cfg.GCPProjectID, "project-id", "nais-local-dev", "Google project ID")
	flag.BoolVar(&cfg.Production, "production", false, "Run in production mode")
	flag.StringVar(&cfg.Env, "env", "dev", "Environment name as defined in Fasit")
}

func main() {
	flag.Parse()

	kubeconfigPath := "" //flag.Lookup("kubeconfig").Value.String()
	if kubeconfigPath == "" {
		if envConfig := os.Getenv("KUBECONFIG"); envConfig != "" {
			kubeconfigPath = envConfig
		} else if home := homedir.HomeDir(); home != "" {
			kubeconfigPath = filepath.Join(home, ".kube", "config")
		}
	}

	fmt.Println("Using", kubeconfigPath)

	// ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	ctx := context.Background()
	// defer cancel()

	log := newLogger()

	var kubeConfig *rest.Config
	var err error

	if cfg.Production {
		kubeConfig, err = rest.InClusterConfig()
		if err != nil {
			log.WithError(err).Fatal("failed to get kubeconfig")
		}
	} else {
		kubeConfig, err = clientcmd.BuildConfigFromFlags("", kubeconfigPath)
		if err != nil {
			panic(err.Error())
		}
	}

	//k8sClient, err := kubernetes.NewForConfig(kubeConfig)
	//if err != nil {
	//	log.WithError(err).Fatal("setting up k8s client")
	//}

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

	informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    addSQLInstance,
		UpdateFunc: updateSQLInstance,
		DeleteFunc: deleteSQLInstance,
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

func deleteSQLInstance(obj any) {
	fmt.Println("delete")
}

func updateSQLInstance(obj any, obj2 any) {
	fmt.Println("update")
}

func addSQLInstance(obj any) {
	fmt.Println("add")
}

func errorHandler(r *cache.Reflector, err error) {
	fmt.Println("watch error ", err)
}

func newLogger() *logrus.Logger {
	log := logrus.StandardLogger()
	log.SetFormatter(&logrus.JSONFormatter{})

	l, err := logrus.ParseLevel(cfg.LogLevel)
	if err != nil {
		log.Fatal(err)
	}
	log.SetLevel(l)
	return log
}

func DefaultConfig() Config {
	return Config{
		BindAddress: ":8080",
		LogLevel:    "info",
	}
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
