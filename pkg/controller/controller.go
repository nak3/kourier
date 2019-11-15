package controller

import (
	"context"
	"flag"
	"kourier/pkg/envoy"
	"kourier/pkg/knative"
	"kourier/pkg/kubernetes"
	"os"
	"path/filepath"

	log "github.com/sirupsen/logrus"

	"knative.dev/serving/pkg/client/listers/networking/v1alpha1"

	corev1listers "k8s.io/client-go/listers/core/v1"
	endpointsinformer "knative.dev/pkg/client/injection/kube/informers/core/v1/endpoints"
	"knative.dev/pkg/configmap"
	"knative.dev/pkg/controller"
	"knative.dev/pkg/logging"
	ingressinformer "knative.dev/serving/pkg/client/injection/informers/networking/v1alpha1/ingress"
)

const (
	controllerName = "KourierController"
	nodeID         = "3scale-kourier-gateway"
	gatewayPort    = 19001
	managementPort = 18000
)

func homeDir() string {
	if h := os.Getenv("HOME"); h != "" {
		return h
	}
	return os.Getenv("USERPROFILE") // windows
}

var kubeconfig string

func init() {
	// Log as JSON instead of the default ASCII formatter.
	log.SetFormatter(&log.JSONFormatter{})

	// Output to stdout instead of the default stderr
	// Can be any io.Writer, see below for File example
	log.SetOutput(os.Stdout)

	// Only log the warning severity or above.
	log.SetLevel(log.InfoLevel)
}

type Reconciler struct {
	IngressLister   v1alpha1.IngressLister
	EndpointsLister corev1listers.EndpointsLister
	KnativeClient   knative.KNativeClient
	EnvoyXDSServer  envoy.EnvoyXdsServer
}

func (r *Reconciler) Reconcile(ctx context.Context, key string) error {
	ingressAccessors, err := r.KnativeClient.IngressAccessors()
	if err != nil {
		return err
	}

	r.EnvoyXDSServer.SetSnapshotForIngresses(nodeID, ingressAccessors)

	return nil
}

func NewController(ctx context.Context, cmw configmap.Watcher) *controller.Impl {
	if home := homeDir(); home != "" {
		_ = flag.Set("kubeconfig", filepath.Join(home, ".kube", "config"))
	} else {
		_ = flag.Set("kubeconfig", "")
	}

	kubeconfig = flag.Lookup("kubeconfig").Value.String()

	config := kubernetes.Config(kubeconfig)
	kubernetesClient := kubernetes.NewKubernetesClient(config)
	knativeClient := knative.NewKnativeClient(config)

	envoyXdsServer := envoy.NewEnvoyXdsServer(gatewayPort, managementPort, kubernetesClient, knativeClient)
	go envoyXdsServer.RunManagementServer()
	go envoyXdsServer.RunGateway()

	logger := logging.FromContext(ctx)

	ingressInformer := ingressinformer.Get(ctx)
	endpointsInformer := endpointsinformer.Get(ctx)

	c := &Reconciler{
		IngressLister:   ingressInformer.Lister(),
		EndpointsLister: endpointsInformer.Lister(),
		KnativeClient:   knativeClient,
		EnvoyXDSServer:  envoyXdsServer,
	}
	impl := controller.NewImpl(c, logger, controllerName)

	ingressInformer.Informer().AddEventHandler(controller.HandleAll(impl.Enqueue))
	endpointsInformer.Informer().AddEventHandler(controller.HandleAll(impl.Enqueue))

	return impl
}
