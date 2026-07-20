package main

import (
	"context"
	stderrors "errors"
	"fmt"
	"os"
	"path/filepath"

	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Errors
var (
	ErrSkipReconcile      = stderrors.New("skip reconcile")
	ErrNoTargetDeployment = stderrors.New("target deployment was not specified")
)

// State
const (
	StateProvisioning = "Provisioning"
	StateReady        = "Ready"
	StateError        = "Error"
)

const (
	sidecarContainerName = "crypto-broker-server"

	socketVolumeName  = "crypto-broker-socket"
	profileVolumeName = "crypto-broker-profile"
	socketMountPath   = "/tmp/open-crypto-broker"
	profileMountPath  = "/app/profiles"

	labelManagedBy      = "app.kubernetes.io/managed-by"
	labelManagedByValue = "crypto-broker-operator"
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

	// Read the local profile file from disk.
	profileContent, err := r.readProfileFile(broker.Spec.Profile)
	if err != nil {
		_ = r.updateStatus(ctx, &broker, StateError)
		return ctrl.Result{}, fmt.Errorf("failed to read profile file: %w", err)
	}

	// Create or Update the ConfigMap.
	cmName := fmt.Sprintf("crypto-broker-profile-%s", broker.Name)
	err = r.reconcileConfigMap(ctx, &broker, cmName, profileContent)
	if err != nil {
		_ = r.updateStatus(ctx, &broker, StateError)
		return ctrl.Result{}, fmt.Errorf("failed to reconcile configmap: %w", err)
	}

	// Get target deployment.
	deployment, err := r.getDeployment(ctx, broker)
	if err != nil {
		broker.Status.State = StateError
		_ = r.Status().Update(ctx, &broker)
		return ctrl.Result{}, err
	}

	// Verify if the profile volume is already declared in the Pod.
	hasVolume := false
	for _, v := range deployment.Spec.Template.Spec.Volumes {
		if v.Name == profileVolumeName && v.ConfigMap != nil && v.ConfigMap.Name == cmName {
			hasVolume = true
			break
		}
	}

	// Check if sidecar is already in the Pod and if it has profile volume.
	if r.hasSidecar(&deployment) && hasVolume {
		broker.Status.State = StateReady
		err := r.Status().Update(ctx, &broker)
		return ctrl.Result{}, err
	}

	// If sidecar already exists, remove it from Pod.
	var containers []core.Container
	for _, c := range deployment.Spec.Template.Spec.Containers {
		if c.Name != sidecarContainerName {
			containers = append(containers, c)
		}
	}
	deployment.Spec.Template.Spec.Containers = containers

	// Inject sidecar.
	r.injectSidecar(&deployment, &broker)

	// Mount volumes.
	r.mountVolumes(&deployment, cmName)

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

func (r *CryptoBrokerReconciler) mountVolumes(deploy *apps.Deployment, configMapName string) {
	// Create socket volume
	socketVolume := core.Volume{
		Name: socketVolumeName,
		VolumeSource: core.VolumeSource{
			EmptyDir: &core.EmptyDirVolumeSource{},
		},
	}

	// Create profiles volume
	profileVolume := core.Volume{
		Name: profileVolumeName,
		VolumeSource: core.VolumeSource{
			ConfigMap: &core.ConfigMapVolumeSource{
				LocalObjectReference: core.LocalObjectReference{
					Name: configMapName,
				},
				Items: []core.KeyToPath{
					{
						Key:  "Profiles.yaml",
						Path: "Profiles.yaml",
					},
				},
			},
		},
	}

	// Declaring volumes in Pod.
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

	// Making sure that every container in the Pod has socket volume.
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
			vol := core.VolumeMount{Name: socketVolumeName, MountPath: socketMountPath}
			*mounts = append(*mounts, vol)
		}
	}
}

func (r *CryptoBrokerReconciler) readProfileFile(profileName string) (string, error) {
	filePath := filepath.Join(r.ProfilesDir, fmt.Sprintf("%s.yaml", profileName))
	content, err := os.ReadFile(filePath)
	if err != nil {
		return "", err
	}

	return string(content), nil
}

func (r *CryptoBrokerReconciler) reconcileConfigMap(ctx context.Context, broker *CryptoBroker, cmName string, content string) error {
	var configMap core.ConfigMap
	nn := types.NamespacedName{Name: cmName, Namespace: broker.Namespace}

	err := r.Get(ctx, nn, &configMap)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// Construct new ConfigMap
			newCM := &core.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      cmName,
					Namespace: broker.Namespace,
					Labels: map[string]string{
						labelManagedBy: labelManagedByValue,
					},
				},
				Data: map[string]string{
					"Profiles.yaml": content,
				},
			}

			if err := ctrl.SetControllerReference(broker, newCM, r.Scheme); err != nil {
				return err
			}

			return r.Create(ctx, newCM)
		}

		return err
	}

	// If it already exists, update data if the file content changed on disk
	if configMap.Data["Profiles.yaml"] != content {
		configMap.Data["Profiles.yaml"] = content
		return r.Update(ctx, &configMap)
	}

	return nil
}

func (r *CryptoBrokerReconciler) updateStatus(ctx context.Context, broker *CryptoBroker, state string) error {
	broker.Status.State = state
	return r.Status().Update(ctx, broker)
}
