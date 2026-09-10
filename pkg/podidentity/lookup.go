/*
Copyright 2026 Joseph Whiteaker.

Licensed under the MIT License. See LICENSE for details.
*/

// Package podidentity looks up the IAM role currently bound to a Kubernetes
// ServiceAccount via an EKS Pod Identity Association.
package podidentity

import (
	"context"
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/eks"
)

// ErrNoAssociation is returned when the ServiceAccount has no EKS Pod
// Identity Association, meaning there is nothing for the controller to track.
var ErrNoAssociation = errors.New("no pod identity association found for service account")

// Lookup resolves the IAM role ARN bound to a namespace/service account pair
// via an EKS Pod Identity Association.
type Lookup interface {
	RoleARN(ctx context.Context, clusterName, namespace, serviceAccount string) (string, error)
}

// EKSAPI is the subset of the EKS SDK client used by EKSLookup, narrowed for
// easier testing.
type EKSAPI interface {
	ListPodIdentityAssociations(ctx context.Context, params *eks.ListPodIdentityAssociationsInput, optFns ...func(*eks.Options)) (*eks.ListPodIdentityAssociationsOutput, error)
	DescribePodIdentityAssociation(ctx context.Context, params *eks.DescribePodIdentityAssociationInput, optFns ...func(*eks.Options)) (*eks.DescribePodIdentityAssociationOutput, error)
}

// EKSLookup implements Lookup using the real EKS API.
type EKSLookup struct {
	API EKSAPI
}

// NewEKSLookup builds an EKSLookup from an eks.Client.
func NewEKSLookup(api *eks.Client) *EKSLookup {
	return &EKSLookup{API: api}
}

// RoleARN returns the IAM role ARN currently associated with the given
// namespace/service account pair on the given EKS cluster. It returns
// ErrNoAssociation if no pod identity association exists.
func (l *EKSLookup) RoleARN(ctx context.Context, clusterName, namespace, serviceAccount string) (string, error) {
	list, err := l.API.ListPodIdentityAssociations(ctx, &eks.ListPodIdentityAssociationsInput{
		ClusterName:    &clusterName,
		Namespace:      &namespace,
		ServiceAccount: &serviceAccount,
	})
	if err != nil {
		return "", fmt.Errorf("listing pod identity associations: %w", err)
	}
	if len(list.Associations) == 0 {
		return "", ErrNoAssociation
	}

	assocID := list.Associations[0].AssociationId

	desc, err := l.API.DescribePodIdentityAssociation(ctx, &eks.DescribePodIdentityAssociationInput{
		ClusterName:   &clusterName,
		AssociationId: assocID,
	})
	if err != nil {
		return "", fmt.Errorf("describing pod identity association %s: %w", *assocID, err)
	}
	if desc.Association == nil || desc.Association.RoleArn == nil {
		return "", ErrNoAssociation
	}

	return *desc.Association.RoleArn, nil
}
