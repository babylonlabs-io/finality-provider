package client

import (
	"fmt"
	"os"
	"strings"
)

// TODO: Decide on which ones to support
const (
	// AWSPrefix is the prefix for AWS Secret Manager references
	AWSPrefix = "arn:aws:secretsmanager:"
	// GCPPrefix is the prefix for Google Cloud Secret Manager references
	GCPPrefix = "projects/"
	// AzurePrefix is the prefix for Azure Key Vault references
	AzurePrefix = "https://"
)

// GetSecretValue retrieves a secret value from various sources based on the reference format
func GetSecretValue(reference string) (string, error) {
	// If it's a reference to a cloud secret manager, fetch the secret
	switch {
	case strings.HasPrefix(reference, AWSPrefix):
		return getAWSSecret(reference)
	case strings.HasPrefix(reference, GCPPrefix) && strings.Contains(reference, "/secrets/"):
		return getGCPSecret(reference)
	case strings.HasPrefix(reference, AzurePrefix) && strings.Contains(reference, ".vault.azure.net/secrets/"):
		return getAzureSecret(reference)
	default:
		return reference, nil
	}
}

// GetHMACKeyWithCloudSupport gets the HMAC key from environment or cloud secret managers
func GetHMACKeyWithCloudSupport() (string, error) {
	keyRef := os.Getenv(HMACKeyEnvVar)
	if keyRef == "" {
		return "", fmt.Errorf("HMAC_KEY environment variable not set")
	}

	return GetSecretValue(keyRef)
}

// getAWSSecret gets a secret from AWS secrets manager
func getAWSSecret(arn string) (string, error) {
	// TODO: Needs to be implemented
	// NOTE: https://pkg.go.dev/github.com/aws/aws-sdk-go-v2/service/secretsmanager
	return "", fmt.Errorf("AWS Secrets Manager integration not implemented: %s", arn)
}

// getGCPSecret gets a secret from GCP secrets manager
func getGCPSecret(secretName string) (string, error) {
	// TODO: Needs to be implemented
	// NOTE: https://cloud.google.com/secret-manager/docs/reference/libraries#client-libraries-install-go
	return "", fmt.Errorf("google cloud secret Manager integration not implemented: %s", secretName)
}

// getAzureSecret gets the secret from Azure Key Vault
func getAzureSecret(secretURI string) (string, error) {
	// TODO: Needs to be implemented
	// NOTE: https://learn.microsoft.com/en-us/azure/key-vault/secrets/quick-create-go?tabs=azure-cli
	return "", fmt.Errorf("azure key vault integration not implemented: %s", secretURI)
}
