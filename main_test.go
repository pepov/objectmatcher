package objectmatcher

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"

	"github.com/goph/emperror"
	v1 "k8s.io/api/core/v1"
	"k8s.io/klog"
)

var (
	integration   = flag.Bool("integration", false, "run integration tests")
	kubeconfig    = flag.String("kubeconfig", filepath.Join(homedir.HomeDir(), ".kube/config"), "kubernetes config to use for tests")
	kubecontext   = flag.String("kubecontext", "", "kubernetes context to use in tests")
	keepnamespace = flag.Bool("keepnamespace", false, "keep the kubernetes namespace that was used for the tests")
	testContext   = &IntegrationTestContext{}
)

func TestMain(m *testing.M) {
	flag.Parse()

	if testing.Verbose() {
		klogFlags := flag.NewFlagSet("klog", flag.ExitOnError)
		klog.InitFlags(klogFlags)
		err := klogFlags.Set("v", "3")
		if err != nil {
			fmt.Printf("Failed to set log level, moving on")
		}
	}

	if *integration {
		err := testContext.Setup()
		if err != nil {
			panic("Failed to setup test namespace")
		}
	}
	result := m.Run()
	if *integration {
		if !*keepnamespace {
			err := testContext.DeleteNamespace()
			if err != nil {
				panic("Failed to delete test namespace")
			}
		}
	}
	os.Exit(result)
}


type IntegrationTestContext struct {
	Client           kubernetes.Interface
	DynamicClient    dynamic.Interface
	Namespace        string
}

func (ctx *IntegrationTestContext) CreateNamespace() error {
	ns := &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "integration-",
		},
	}
	namespace, err := ctx.Client.CoreV1().Namespaces().Create(ns)
	if err != nil {
		return err
	}
	ctx.Namespace = namespace.Name
	return nil
}

func (ctx *IntegrationTestContext) Setup() error {
	config, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		&clientcmd.ClientConfigLoadingRules{ExplicitPath: *kubeconfig},
		&clientcmd.ConfigOverrides{CurrentContext: *kubecontext},
	).ClientConfig()
	if err != nil {
		return err
	}
	ctx.Client, err = kubernetes.NewForConfig(config)
	if err != nil {
		return emperror.Wrap(err, "Failed to create kubernetes client")
	}
	ctx.DynamicClient, err = dynamic.NewForConfig(config)
	if err != nil {
		return emperror.Wrap(err, "Failed to create dynamic client")
	}
	err = testContext.CreateNamespace()
	if err != nil {
		return emperror.Wrap(err, "Failed to create test namespace")
	}
	return err
}

func (ctx *IntegrationTestContext) DeleteNamespace() error {
	err := ctx.Client.CoreV1().Namespaces().Delete(ctx.Namespace, &metav1.DeleteOptions{
		GracePeriodSeconds: new(int64),
	})
	return err
}
