package main

import (
	"context"
	stderrors "errors"
	"fmt"
	"os"
	"path/filepath"

	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
)

var (
	ErrNoTargetDeployment = stderrors.New("target deployment was not specified")
	ErrNoProfileFiles     = stderrors.New("failed to read profile files")
	ErrTargetNotFound     = stderrors.New("target deployment cannot be found")
	ErrProfileConfigMap   = stderrors.New("failed to ensure profile config map")
)

var logger = ctrl.Log.WithName("crypto-broker-operator")

const (
	StateProvisioning = "Provisioning"
	StateReady        = "Ready"
	StateError        = "Error"

	sidecarContainerName = "crypto-broker-server"

	socketVolumeName  = "crypto-broker-server"
	profileVolumeName = "crypto-broker-profile"

	socketMountPath  = "/tmp/open-crypto-broker"
	profileMountPath = "/app/profiles"
)

type CryptoBrokerReconciler struct {
	client.Client
	Scheme      *runtime.Scheme
	ProfilesDir string
	ServerImage string
}

func (r *CryptoBrokerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// Load crypto broker
	broker, err := r.loadCryptoBroker(ctx, req)
	if err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Set initial status
	err = r.patchStatus(ctx, &broker, StateProvisioning, "Initiating crypto broker injection", nil)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Read profile
	profileContent, err := r.readProfileFile(broker.Spec.Profile)
	if err != nil {
		_ = r.patchStatus(ctx, &broker, StateError, err.Error(), nil)
		return ctrl.Result{}, err
	}

	// Get target deployment
	deployment, err := r.getDeployment(ctx, broker)
	if err != nil {
		err = fmt.Errorf("%w: %s", ErrTargetNotFound, broker.Spec.TargetDeployment)

		_ = r.patchStatus(ctx, &broker, StateError, err.Error(), nil)
		return ctrl.Result{}, err
	}

	// Ensure we always have config map with our profile
	configMapName := fmt.Sprintf("crypto-broker-profile-%s", broker.Name)
	err = r.ensureProfileConfigMap(ctx, configMapName, broker.Namespace, profileContent)
	if err != nil {
		_ = r.patchStatus(ctx, &broker, StateError, err.Error(), nil)
		return ctrl.Result{}, err
	}

	// Inject sidecar
	r.injectSidecar(&deployment, &broker)

	// Ensure we have volumes attached
	r.ensureVolumes(&deployment, configMapName)

	// Update deployment
	err = r.Update(ctx, &deployment)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Get profile details
	details, readyMsg, err := getProfileDetails(profileContent, broker.Spec.Profile)
	if err != nil {
		_ = r.patchStatus(ctx, &broker, StateError, err.Error(), nil)
		return ctrl.Result{}, err
	}

	err = r.patchStatus(ctx, &broker, StateReady, readyMsg, details)
	return ctrl.Result{}, err
}

func getProfileDetails(profileContent string, configuredProfile string) (*ProfileDetails, string, error) {
	var entries []ProfileEntry

	err := yaml.Unmarshal([]byte(profileContent), &entries)
	if err != nil {
		return nil, "", err
	}

	readyMessage := "Crypto broker is running"

	if len(entries) == 0 {
		return nil, readyMessage, nil
	}

	entry := entries[0]

	details := &ProfileDetails{
		HashAlgorithm: entry.API.HashData.HashAlg,
		SignAlgorithm: entry.API.SignCertificate.SignAlg,
	}

	if entry.Name != "" && entry.Name != configuredProfile {
		readyMessage = fmt.Sprintf("Crypto broker is running profile %q", entry.Name)
	}

	return details, readyMessage, nil
}

func (r *CryptoBrokerReconciler) getDeployment(ctx context.Context, broker CryptoBroker) (apps.Deployment, error) {
	var deployment apps.Deployment

	err := r.Get(
		ctx,
		types.NamespacedName{
			Name:      broker.Spec.TargetDeployment,
			Namespace: broker.Namespace,
		},
		&deployment,
	)

	return deployment, err
}

func (r *CryptoBrokerReconciler) loadCryptoBroker(ctx context.Context, req ctrl.Request) (CryptoBroker, error) {
	var broker CryptoBroker

	err := r.Get(ctx, req.NamespacedName, &broker)
	if err != nil {
		return broker, err
	}

	if broker.Spec.TargetDeployment == "" {
		_ = r.patchStatus(ctx, &broker, StateError, ErrNoTargetDeployment.Error(), nil)
		return broker, ErrNoTargetDeployment
	}

	if broker.Spec.Profile == "" {
		_ = r.patchStatus(ctx, &broker, StateError, ErrNoProfileFiles.Error(), nil)
		return broker, ErrNoProfileFiles
	}

	return broker, nil
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

	sidecar := core.Container{
		Name:            sidecarContainerName,
		Image:           image,
		ImagePullPolicy: core.PullIfNotPresent,

		Env: []core.EnvVar{
			{
				Name:  "CRYPTO_BROKER_PROFILES_DIR",
				Value: profileMountPath,
			},
			{
				Name:  "CRYPTO_BROKER_APP_ENV",
				Value: env,
			},
		},

		VolumeMounts: []core.VolumeMount{
			{
				Name:      socketVolumeName,
				MountPath: socketMountPath,
			},
			{
				Name:      profileVolumeName,
				MountPath: profileMountPath,
				ReadOnly:  true,
			},
		},

		SecurityContext: &core.SecurityContext{
			RunAsNonRoot:             &truePtr,
			RunAsUser:                &userPtr,
			ReadOnlyRootFilesystem:   &truePtr,
			AllowPrivilegeEscalation: &falsePtr,
		},
	}

	for i := range deploy.Spec.Template.Spec.Containers {
		if deploy.Spec.Template.Spec.Containers[i].Name == sidecarContainerName {
			deploy.Spec.Template.Spec.Containers[i] = sidecar
			return
		}
	}

	deploy.Spec.Template.Spec.Containers = append(
		deploy.Spec.Template.Spec.Containers,
		sidecar,
	)
}

func (r *CryptoBrokerReconciler) ensureVolumes(deploy *apps.Deployment, configMapName string) {
	socketVolume := core.Volume{
		Name: socketVolumeName,
		VolumeSource: core.VolumeSource{
			EmptyDir: &core.EmptyDirVolumeSource{},
		},
	}

	profileVolume := core.Volume{
		Name: profileVolumeName,
		VolumeSource: core.VolumeSource{
			ConfigMap: &core.ConfigMapVolumeSource{
				LocalObjectReference: core.LocalObjectReference{
					Name: configMapName,
				},
			},
		},
	}

	volumes := &deploy.Spec.Template.Spec.Volumes

	socketFound := false
	profileFound := false

	for i := range *volumes {
		switch (*volumes)[i].Name {
		case socketVolumeName:
			(*volumes)[i] = socketVolume
			socketFound = true

		case profileVolumeName:
			(*volumes)[i] = profileVolume
			profileFound = true
		}
	}

	if !socketFound {
		*volumes = append(*volumes, socketVolume)
	}

	if !profileFound {
		*volumes = append(*volumes, profileVolume)
	}

	for i := range deploy.Spec.Template.Spec.Containers {
		container := &deploy.Spec.Template.Spec.Containers[i]

		found := false

		for _, mount := range container.VolumeMounts {
			if mount.Name == socketVolumeName {
				found = true
				break
			}
		}

		if found {
			continue
		}

		container.VolumeMounts = append(
			container.VolumeMounts,
			core.VolumeMount{
				Name:      socketVolumeName,
				MountPath: socketMountPath,
			},
		)
	}
}

func (r *CryptoBrokerReconciler) readProfileFile(profileName string) (string, error) {
	filePath := filepath.Join(
		r.ProfilesDir,
		fmt.Sprintf("%s.yaml", profileName),
	)

	content, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrNoProfileFiles, err)
	}

	return string(content), nil
}

func (r *CryptoBrokerReconciler) patchStatus(ctx context.Context, broker *CryptoBroker, status string, message string, profileDetails *ProfileDetails) error {
	base := broker.DeepCopy()

	broker.Status.State = status
	broker.Status.SocketPath = fmt.Sprintf("%s/%s.sock", socketMountPath, socketVolumeName)
	broker.Status.Message = message

	if profileDetails != nil {
		broker.Status.ProfileDetails = profileDetails
	}

	err := r.Status().Patch(ctx, broker, client.MergeFrom(base))

	if err != nil {
		logger.Error(err, "failed to patch status", "namespace", broker.Namespace, "name", broker.Name)
	}

	return err
}

func (r *CryptoBrokerReconciler) ensureProfileConfigMap(ctx context.Context, name string, namespace string, content string) error {
	var configMap core.ConfigMap

	err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, &configMap)
	if err != nil {
		if client.IgnoreNotFound(err) != nil {
			return fmt.Errorf("%w: %v", ErrProfileConfigMap, err)
		}

		configMap = core.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
			Data: map[string]string{
				"Profiles.yaml": content,
			},
		}

		err := r.Create(ctx, &configMap)
		if err != nil {
			return fmt.Errorf("%w: %v", ErrProfileConfigMap, err)
		}

		return nil
	}

	if configMap.Data["Profiles.yaml"] == content {
		return nil
	}

	configMap.Data["Profiles.yaml"] = content
	err = r.Update(ctx, &configMap)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrProfileConfigMap, err)
	}

	return nil
}
