// Unless explicitly stated otherwise all files in this repository are licensed under the MIT License.
//
// This product includes software developed at Datadog (https://www.datadoghq.com/). Copyright 2024 Datadog, Inc.

package v1alpha1

import (
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

func (r *TemporalWorkerDeployment) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

//+kubebuilder:webhook:path=/mutate-temporal-io-temporal-io-v1alpha1-temporalworkerdeployment,mutating=true,failurePolicy=fail,sideEffects=None,groups=temporal.io.temporal.io,resources=temporalworkers,verbs=create;update,versions=v1alpha1,name=mtemporalworker.kb.io,admissionReviewVersions=v1

var _ webhook.Defaulter = &TemporalWorkerDeployment{}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (r *TemporalWorkerDeployment) Default() {
}
