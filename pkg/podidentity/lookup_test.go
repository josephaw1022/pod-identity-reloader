/*
Copyright 2026 Joseph Whiteaker.

Licensed under the MIT License. See LICENSE for details.
*/

package podidentity

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/eks"
	"github.com/aws/aws-sdk-go-v2/service/eks/types"
)

type fakeEKSAPI struct {
	listOutput  *eks.ListPodIdentityAssociationsOutput
	listErr     error
	describeOut *eks.DescribePodIdentityAssociationOutput
	describeErr error
	lastListIn  *eks.ListPodIdentityAssociationsInput
	lastDescIn  *eks.DescribePodIdentityAssociationInput
}

func (f *fakeEKSAPI) ListPodIdentityAssociations(
	_ context.Context, params *eks.ListPodIdentityAssociationsInput, _ ...func(*eks.Options),
) (*eks.ListPodIdentityAssociationsOutput, error) {
	f.lastListIn = params
	return f.listOutput, f.listErr
}

func (f *fakeEKSAPI) DescribePodIdentityAssociation(
	_ context.Context, params *eks.DescribePodIdentityAssociationInput, _ ...func(*eks.Options),
) (*eks.DescribePodIdentityAssociationOutput, error) {
	f.lastDescIn = params
	return f.describeOut, f.describeErr
}

func strPtr(s string) *string { return &s }

func TestEKSLookupRoleARN(t *testing.T) {
	t.Run("returns the role arn for the first association", func(t *testing.T) {
		api := &fakeEKSAPI{
			listOutput: &eks.ListPodIdentityAssociationsOutput{
				Associations: []types.PodIdentityAssociationSummary{
					{AssociationId: strPtr("assoc-1")},
				},
			},
			describeOut: &eks.DescribePodIdentityAssociationOutput{
				Association: &types.PodIdentityAssociation{
					RoleArn: strPtr("arn:aws:iam::111122223333:role/my-role"),
				},
			},
		}
		lookup := NewEKSLookup(nil)
		lookup.API = api

		got, err := lookup.RoleARN(context.Background(), "my-cluster", "default", "my-sa")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "arn:aws:iam::111122223333:role/my-role" {
			t.Fatalf("unexpected role arn: %q", got)
		}
		if api.lastListIn == nil || *api.lastListIn.ClusterName != "my-cluster" ||
			*api.lastListIn.Namespace != "default" || *api.lastListIn.ServiceAccount != "my-sa" {
			t.Fatalf("ListPodIdentityAssociations called with unexpected input: %+v", api.lastListIn)
		}
		if api.lastDescIn == nil || *api.lastDescIn.AssociationId != "assoc-1" {
			t.Fatalf("DescribePodIdentityAssociation called with unexpected input: %+v", api.lastDescIn)
		}
	})

	t.Run("returns ErrNoAssociation when nothing is associated", func(t *testing.T) {
		api := &fakeEKSAPI{
			listOutput: &eks.ListPodIdentityAssociationsOutput{},
		}
		lookup := NewEKSLookup(nil)
		lookup.API = api

		_, err := lookup.RoleARN(context.Background(), "my-cluster", "default", "my-sa")
		if !errors.Is(err, ErrNoAssociation) {
			t.Fatalf("expected ErrNoAssociation, got: %v", err)
		}
	})

	t.Run("wraps list errors", func(t *testing.T) {
		api := &fakeEKSAPI{listErr: errors.New("boom")}
		lookup := NewEKSLookup(nil)
		lookup.API = api

		_, err := lookup.RoleARN(context.Background(), "my-cluster", "default", "my-sa")
		if err == nil {
			t.Fatal("expected an error")
		}
	})

	t.Run("wraps describe errors", func(t *testing.T) {
		api := &fakeEKSAPI{
			listOutput: &eks.ListPodIdentityAssociationsOutput{
				Associations: []types.PodIdentityAssociationSummary{{AssociationId: strPtr("assoc-1")}},
			},
			describeErr: errors.New("boom"),
		}
		lookup := NewEKSLookup(nil)
		lookup.API = api

		_, err := lookup.RoleARN(context.Background(), "my-cluster", "default", "my-sa")
		if err == nil {
			t.Fatal("expected an error")
		}
	})
}
