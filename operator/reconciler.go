package main

import (
	"context"
	stderrors "errors"
	"fmt"

	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var ErrSkipReconcile = stderrors.New("skip reconcile")

const (
	StateReady = "Ready"
	StateError = "Error"

	sidecarContainerName = "crypto-broker-server"

	socketVolumeName  = "crypto-broker-socket"
	profileVolumeName = "crypto-broker-profile"
	socketMountPath   = "/tmp/open-crypto-broker"
	profileMountPath  = "/app/profiles"

	labelManagedBy      = "app.kubernetes.io/managed-by"
	labelManagedByValue = "crypto-broker-operator"

	configMapProfile   = "crypto-broker-profile-catalog"
	configMapNamespace = "default"
)

type CryptoBrokerReconciler struct {
	client.Client

	Scheme      *runtime.Scheme
	ProfilesDir string
	ServerImage string
}

func (r *CryptoBrokerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// Load crypto broker.
	broker, err := r.loadCryptoBroker(ctx, req)
	if err != nil {
		if stderrors.Is(err, ErrSkipReconcile) {
			return ctrl.Result{}, nil
		}

		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Ensure we have configMap with our profiles.
	err = r.hasConfigProfile(ctx, configMapProfile)
	if err != nil {
		broker.Status.State = StateError
		_ = r.Status().Update(ctx, &broker)
		return ctrl.Result{}, err
	}

	// Get target deployment.
	deployment, err := r.getDeployment(ctx, broker)
	if err != nil {
		broker.Status.State = StateError
		_ = r.Status().Update(ctx, &broker)
		return ctrl.Result{}, err
	}

	// Check if sidecar is already in the Pod.
	if r.hasSidecar(&deployment) {
		broker.Status.State = StateReady
		err := r.Status().Update(ctx, &broker)
		return ctrl.Result{}, err
	}

	// Mount volumes.
	r.mountVolumes(&deployment, broker.Spec.Profile)

	// Inject sidecar.
	r.injectSidecar(&deployment, &broker)

	// Update deployment and mark broker as ready.
	err = r.Update(ctx, &deployment)
	if err != nil {
		return ctrl.Result{}, err
	}

	broker.Status.State = StateReady
	err = r.Status().Update(ctx, &broker)
	if err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *CryptoBrokerReconciler) hasConfigProfile(ctx context.Context, profileName string) error {
	var configMap core.ConfigMap
	name := types.NamespacedName{Name: profileName, Namespace: configMapNamespace}

	return r.Get(ctx, name, &configMap)
}

func (r *CryptoBrokerReconciler) getDeployment(ctx context.Context, broker CryptoBroker) (apps.Deployment, error) {
	var deployment apps.Deployment

	deployKey := types.NamespacedName{
		Name:      broker.Spec.TargetDeployment,
		Namespace: broker.Namespace,
	}

	err := r.Get(ctx, deployKey, &deployment)
	return deployment, err
}

func (r *CryptoBrokerReconciler) loadCryptoBroker(ctx context.Context, req ctrl.Request) (CryptoBroker, error) {
	var broker CryptoBroker

	err := r.Get(ctx, req.NamespacedName, &broker)
	if err != nil {
		return broker, err
	}

	if broker.Status.State == StateReady {
		return broker, ErrSkipReconcile
	}

	if broker.Spec.TargetDeployment == "" {
		broker.Status.State = StateError
		_ = r.Status().Update(ctx, &broker)

		return broker, ErrSkipReconcile
	}

	return broker, nil
}

func (r *CryptoBrokerReconciler) hasSidecar(deploy *apps.Deployment) bool {
	for _, container := range deploy.Spec.Template.Spec.Containers {
		if container.Name == sidecarContainerName {
			return true
		}
	}

	return false
}

func (r *CryptoBrokerReconciler) injectSidecar(deploy *apps.Deployment, broker *CryptoBroker) {
	truePtr := true
	falsePtr := false
	userPtr := int64(1000)

	env := "prod"
	if broker.Spec.Environment != "" {
		env = broker.Spec.Environment
	}

	image := r.ServerImage
	if broker.Spec.Version != "" && broker.Spec.Version != "latest" {
		image = fmt.Sprintf("ghcr.io/open-crypto-broker/server:%s", broker.Spec.Version)
	}

	// Create sidecar.
	sidecar := core.Container{
		Name:            sidecarContainerName,
		Image:           image,
		ImagePullPolicy: core.PullIfNotPresent,

		Env: []core.EnvVar{
			{Name: "CRYPTO_BROKER_PROFILES_DIR", Value: profileMountPath},
			{Name: "CRYPTO_BROKER_APP_ENV", Value: env},
		},

		VolumeMounts: []core.VolumeMount{
			{Name: socketVolumeName, MountPath: socketMountPath},
			{Name: profileVolumeName, MountPath: profileMountPath, ReadOnly: true},
		},

		SecurityContext: &core.SecurityContext{
			RunAsNonRoot:             &truePtr,
			RunAsUser:                &userPtr,
			ReadOnlyRootFilesystem:   &truePtr,
			AllowPrivilegeEscalation: &falsePtr,
		},
	}

	// Inject sidecar into Pod.
	deploy.Spec.Template.Spec.Containers = append(
		deploy.Spec.Template.Spec.Containers,
		sidecar,
	)
}

func (r *CryptoBrokerReconciler) mountVolumes(deploy *apps.Deployment, profile string) {
	// Create shared socket volume.
	socketVolume := core.Volume{
		Name: socketVolumeName,
		VolumeSource: core.VolumeSource{
			EmptyDir: &core.EmptyDirVolumeSource{},
		},
	}

	// Create profile ConfigMap volume.
	profileVolume := core.Volume{
		Name: profileVolumeName,
		VolumeSource: core.VolumeSource{
			ConfigMap: &core.ConfigMapVolumeSource{
				LocalObjectReference: core.LocalObjectReference{
					Name: configMapProfile,
				},
				Items: []core.KeyToPath{
					{
						Key:  profile + ".yaml",
						Path: "Profiles.yaml",
					},
				},
			},
		},
	}

	// Add volumes to the Pod.
	volumes := &deploy.Spec.Template.Spec.Volumes
	volumeMap := make(map[string]bool)

	for _, v := range *volumes {
		volumeMap[v.Name] = true
	}

	if !volumeMap[socketVolumeName] {
		*volumes = append(*volumes, socketVolume)
	}

	if !volumeMap[profileVolumeName] {
		*volumes = append(*volumes, profileVolume)
	}

	// Make sure that shared socket volume exists in every container in the Pod.
	for i := range deploy.Spec.Template.Spec.Containers {
		mounts := &deploy.Spec.Template.Spec.Containers[i].VolumeMounts

		found := false
		for _, m := range *mounts {
			if m.Name == socketVolumeName {
				found = true
				break
			}
		}

		if !found {
			vol := core.VolumeMount{
				Name:      socketVolumeName,
				MountPath: socketMountPath,
			}

			*mounts = append(*mounts, vol)
		}
	}
}
