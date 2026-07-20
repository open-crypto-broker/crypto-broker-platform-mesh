package main

import (
	"log"
	"log/slog"
	"os"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
)

var scheme = runtime.NewScheme()

func init() {
	utilruntime.Must(corev1.AddToScheme(scheme))
	utilruntime.Must(appsv1.AddToScheme(scheme))
	utilruntime.Must(AddToScheme(scheme))
}

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	ctrl.SetLogger(logr.FromSlogHandler(logger.Handler()))

	profilesDir := os.Getenv("PROFILES_CATALOG_DIR")
	if profilesDir == "" {
		profilesDir = "/profiles"
	}

	serverImage := os.Getenv("CRYPTO_BROKER_SERVER_IMAGE")
	if serverImage == "" {
		serverImage = "ghcr.io/open-crypto-broker/server:latest"
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{Scheme: scheme})
	if err != nil {
		log.Fatalf("unable to start manager: %v", err)
	}

	err = ctrl.NewControllerManagedBy(mgr).
		For(&CryptoBroker{}).
		Complete(&CryptoBrokerReconciler{
			Client:      mgr.GetClient(),
			Scheme:      mgr.GetScheme(),
			ProfilesDir: profilesDir,
			ServerImage: serverImage,
		})

	if err != nil {
		log.Fatalf("unable to create controller: %v", err)
	}

	err = mgr.Start(ctrl.SetupSignalHandler())
	if err != nil {
		log.Fatalf("problem running manager: %v", err)
	}
}
