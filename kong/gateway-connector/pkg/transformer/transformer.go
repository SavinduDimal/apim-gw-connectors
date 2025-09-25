/*
 *  Copyright (c) 2025, WSO2 LLC. (http://www.wso2.org) All Rights Reserved.
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *  http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 *
 */

package transformer

import (
	"strings"

	v1 "github.com/kong/kubernetes-configuration/api/configuration/v1"
	"github.com/wso2-extensions/apim-gw-connectors/common-agent/config"
	eventHub "github.com/wso2-extensions/apim-gw-connectors/common-agent/pkg/eventhub/types"
	"github.com/wso2-extensions/apim-gw-connectors/common-agent/pkg/k8s-resource-lib/constants"
	httpGenerator "github.com/wso2-extensions/apim-gw-connectors/common-agent/pkg/k8s-resource-lib/pkg/generators/http"
	"github.com/wso2-extensions/apim-gw-connectors/common-agent/pkg/k8s-resource-lib/pkg/utils"
	"github.com/wso2-extensions/apim-gw-connectors/common-agent/pkg/k8s-resource-lib/types"
	apimTransformer "github.com/wso2-extensions/apim-gw-connectors/common-agent/pkg/transformer"
	kongConstants "github.com/wso2-extensions/apim-gw-connectors/kong/gateway-connector/constants"
	logger "github.com/wso2-extensions/apim-gw-connectors/kong/gateway-connector/internal/loggers"
	kongMgtServer "github.com/wso2-extensions/apim-gw-connectors/kong/gateway-connector/pkg/managementserver"
	"gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// UpdateCRS updates the Kubernetes custom resources with environment-specific metadata and labels.
func UpdateCRS(k8sArtifact *K8sArtifacts, environments *[]apimTransformer.Environment, organizationID string, apiUUID string, apiName string, revisionID string, namespace string, configuredRateLimitPoliciesMap map[string]eventHub.RateLimitPolicy) {
	logger.LoggerUtils.Debugf("UpdateCRS|Starting CR update|API:%s Revision:%s Environments:%d\n",
		apiUUID, revisionID, len(*environments))

	organizationHash := GenerateSHA1Hash(organizationID)

	for _, httproute := range k8sArtifact.HTTPRoutes {
		httproute.ObjectMeta.Labels[kongConstants.OrganizationLabel] = organizationHash
		httproute.ObjectMeta.Labels[kongConstants.APIUUIDLabel] = apiUUID
		httproute.ObjectMeta.Labels[kongConstants.RevisionIDLabel] = revisionID
		httproute.ObjectMeta.Labels[kongConstants.APINameLabel] = apiName
		httproute.ObjectMeta.Labels[kongConstants.K8sInitiatedFromField] = kongConstants.ControlPlaneOrigin

		for _, environment := range *environments {
			vhost := environment.Vhost

			if httproute.ObjectMeta.Labels[kongConstants.EnvironmentLabel] == constants.ProductionType {
				httproute.Spec.Hostnames = []gwapiv1.Hostname{gwapiv1.Hostname(vhost)}
			}
			if httproute.ObjectMeta.Labels[kongConstants.EnvironmentLabel] == constants.SandboxType {
				httproute.Spec.Hostnames = []gwapiv1.Hostname{gwapiv1.Hostname(kongConstants.SandboxHostPrefix + vhost)}
			}
		}
	}
	for _, service := range k8sArtifact.Services {
		service.ObjectMeta.Labels = make(map[string]string)
		service.ObjectMeta.Labels[kongConstants.OrganizationLabel] = organizationHash
		service.ObjectMeta.Labels[kongConstants.APIUUIDLabel] = apiUUID
		service.ObjectMeta.Labels[kongConstants.RevisionIDLabel] = revisionID
		service.ObjectMeta.Labels[kongConstants.APINameLabel] = apiName
		service.ObjectMeta.Labels[kongConstants.K8sInitiatedFromField] = kongConstants.ControlPlaneOrigin
	}
	for _, kongPlugin := range k8sArtifact.KongPlugins {
		kongPlugin.ObjectMeta.Labels = make(map[string]string)
		kongPlugin.ObjectMeta.Labels[kongConstants.OrganizationLabel] = organizationHash
		kongPlugin.ObjectMeta.Labels[kongConstants.APIUUIDLabel] = apiUUID
		kongPlugin.ObjectMeta.Labels[kongConstants.RevisionIDLabel] = revisionID
		kongPlugin.ObjectMeta.Labels[kongConstants.APINameLabel] = apiName
		kongPlugin.ObjectMeta.Labels[kongConstants.K8sInitiatedFromField] = kongConstants.ControlPlaneOrigin
	}
}

// CreateConsumer handles the Kong consumer generation
func CreateConsumer(applicationUUID string, environment string, conf *config.Config) *v1.KongConsumer {
	logger.LoggerUtils.Debugf("Creating Kong consumer|App:%s Env:%s\n", applicationUUID, environment)

	ingressClassName := conf.DataPlane.GatewayClassName
	if ingressClassName == kongConstants.EmptyString {
		ingressClassName = kongConstants.DefaultIngressClassName
	}
	consumer := v1.KongConsumer{
		TypeMeta: metav1.TypeMeta{
			Kind:       kongConstants.KongConsumerKind,
			APIVersion: kongConstants.KongAPIVersion,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: GenerateConsumerName(applicationUUID, environment),
			Annotations: map[string]string{
				kongConstants.KubernetesIngressClass: ingressClassName,
			},
			Labels: make(map[string]string, 0),
		},
		Username: GenerateSHA1Hash(applicationUUID + environment),
	}
	consumer.Labels[kongConstants.ApplicationUUIDLabel] = applicationUUID
	if environment != kongConstants.EmptyString {
		consumer.Labels[kongConstants.EnvironmentLabel] = environment
	}
	return &consumer
}

// GenerateK8sCredentialSecret handles the k8s secret generation for kong credentials
func GenerateK8sCredentialSecret(applicationUUID string, identifier string, credentialName string, data map[string]string) *corev1.Secret {
	logger.LoggerUtils.Debugf("Generating credential secret|App:%s Credential:%s\n",
		applicationUUID, credentialName)

	secret := corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       kongConstants.SecretKind,
			APIVersion: kongConstants.CoreAPIVersion,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: GenerateSecretName(applicationUUID, identifier, credentialName),
			Labels: map[string]string{
				kongConstants.KongCredentialLabel: credentialName,
			},
		},
		StringData: data,
	}
	secret.Labels[kongConstants.ApplicationUUIDLabel] = applicationUUID
	return &secret
}

// GenerateK8sSecret handles the k8s secret generation
func GenerateK8sSecret(name string, organization string, labels map[string]string, data map[string]string) *corev1.Secret {
	logger.LoggerUtils.Debugf("Generating k8s secret|Name:%s Labels:%d\n", name, len(labels))

	secret := corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       kongConstants.SecretKind,
			APIVersion: kongConstants.CoreAPIVersion,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:   PrepareDashedName(name + kongConstants.DashSeparatorString + organization),
			Labels: labels,
		},
		StringData: data,
	}
	return &secret
}

// GenerateKongPlugin handles the Kong plugin generation
func GenerateKongPlugin(operation *types.Operation, pluginName string, targetRef string, config KongPluginConfig, enabled bool) *v1.KongPlugin {
	logger.LoggerUtils.Debugf("Generating Kong plugin|Plugin:%s Enabled:%v\n", pluginName, enabled)

	return &v1.KongPlugin{
		TypeMeta: metav1.TypeMeta{
			Kind:       kongConstants.KongPluginKind,
			APIVersion: kongConstants.KongAPIVersion,
		},
		PluginName: pluginName,
		ObjectMeta: metav1.ObjectMeta{
			Name: GeneratePluginCRName(operation, targetRef, pluginName),
		},
		Disabled: !enabled,
		Config: apiextensionsv1.JSON{
			Raw: GenerateJSON(config),
		},
	}
}

func CreateIssuerKongSecretCredential(issuerSecret corev1.Secret, conf *config.Config, applicationUUID string, consumerKey string, environment string) *corev1.Secret {
	logger.LoggerEvents.Debugf("Creating issuer Kong secret credential for ApplicationUUID: %s, Environment: %s", applicationUUID, environment)

	rsaPublicKey, exists := issuerSecret.Data[kongConstants.PublicKeyField]
	if !exists {
		logger.LoggerEvents.Errorf("Public key not found in issuer secret")
		return nil
	}

	jwtCredentialSecretConfig := map[string]string{
		kongConstants.AlgorithmField:    kongConstants.RS256Algorithm,
		kongConstants.KeyField:          consumerKey,
		kongConstants.RSAPublicKeyField: string(rsaPublicKey),
	}

	jwtCredentialSecret := GenerateK8sCredentialSecret(applicationUUID, consumerKey, kongConstants.JWTCredentialType, jwtCredentialSecretConfig)

	if jwtCredentialSecret.Labels == nil {
		jwtCredentialSecret.Labels = make(map[string]string, 1)
	}
	jwtCredentialSecret.Labels[kongConstants.EnvironmentLabel] = strings.ToLower(environment)
	jwtCredentialSecret.Namespace = conf.DataPlane.Namespace

	return jwtCredentialSecret
}
