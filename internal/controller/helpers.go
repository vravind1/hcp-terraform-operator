// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	tfc "github.com/hashicorp/go-tfe"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	legacyVersionThreshold = 2024091  // existing threshold for legacy versions >= v202409-1
	semanticVersionBase    = 300000000 // any semantic encoded value starts at or above this
)

func doNotRequeue() (reconcile.Result, error) {
	return reconcile.Result{}, nil
}

func requeueAfter(duration time.Duration) (reconcile.Result, error) {
	return reconcile.Result{Requeue: true, RequeueAfter: duration}, nil
}

func requeueOnErr(err error) (reconcile.Result, error) {
	return reconcile.Result{}, err
}

// formatOutput formats TFC/E output to a string or bytes to save it further in
// Kubernetes ConfigMap or Secret, respectively.
//
// Terraform supports the following types:
// - https://developer.hashicorp.com/terraform/language/expressions/types
// When the output value is `null`(special value), TFC/E does not return it.
// Thus, we do not catch it here.
func formatOutput(o *tfc.StateVersionOutput) (string, error) {
	switch x := o.Value.(type) {
	case bool:
		return strconv.FormatBool(x), nil
	case float64:
		return fmt.Sprint(x), nil
	case string:
		return x, nil
	default:
		b, err := json.Marshal(o.Value)
		if err != nil {
			return "", err
		}
		return string(b), nil
	}
}

type Object interface {
	client.Object
}

// needToAddFinalizer reports true when a given object doesn't contain a given finalizer and it is not marked for deletion.
// Otherwise, it reports false.
func needToAddFinalizer[T Object](o T, finalizer string) bool {
	return o.GetDeletionTimestamp().IsZero() && !controllerutil.ContainsFinalizer(o, finalizer)
}

// isDeletionCandidate reports true when a given object contains a given finalizer and it is marked for deletion.
// Otherwise, it reports false.
func isDeletionCandidate[T Object](o T, finalizer string) bool {
	return !o.GetDeletionTimestamp().IsZero() && controllerutil.ContainsFinalizer(o, finalizer)
}

// configMapKeyRef fetches a given key name from a given Kubernetes Config Map.
func configMapKeyRef(ctx context.Context, c client.Client, nn types.NamespacedName, key string) (string, error) {
	cm := &corev1.ConfigMap{}
	if err := c.Get(ctx, nn, cm); err != nil {
		return "", err
	}

	if k, ok := cm.Data[key]; ok {
		return k, nil
	}

	return "", fmt.Errorf("unable to find key=%q in configMap=%q namespace=%q", key, nn.Name, nn.Namespace)
}

// secretKeyRef fetches a given key name from a given Kubernetes Secret.
func secretKeyRef(ctx context.Context, c client.Client, nn types.NamespacedName, key string) (string, error) {
	secret := &corev1.Secret{}
	if err := c.Get(ctx, nn, secret); err != nil {
		return "", err
	}

	if k, ok := secret.Data[key]; ok {
		return strings.TrimSpace(string(k)), nil
	}

	return "", fmt.Errorf("unable to find key=%q in secret=%q namespace=%q", key, nn.Name, nn.Namespace)
}

// parseTFEVersionDetailed parses TFE version strings in both legacy (vYYYYMM-N) and 
// semantic (MAJOR.MINOR.PATCH[...]) formats.
//
// For legacy versions (e.g., v202409-1), it returns the composed numeric value (2024091) 
// and isSemantic=false.
//
// For semantic versions (e.g., 1.2.3), it encodes them as:
// semanticVersionBase + major*1_000_000 + minor*1_000 + patch
// and returns isSemantic=true.
//
// This encoding ensures semantic versions are always >= semanticVersionBase (300000000),
// making them easily distinguishable from legacy versions.
func parseTFEVersionDetailed(version string) (int, bool, error) {
	// Try legacy format first: ^v(\d{6})-(\d)$
	legacyRegexp := regexp.MustCompile(`^v([0-9]{6})-([0-9]{1})$`)
	matches := legacyRegexp.FindStringSubmatch(version)
	if len(matches) == 3 {
		versionNum, err := strconv.Atoi(matches[1] + matches[2])
		if err != nil {
			return 0, false, fmt.Errorf("failed to parse legacy TFE version %s: %v", version, err)
		}
		return versionNum, false, nil
	}

	// Try semantic format: ^(\d+)\.(\d+)\.(\d+)(?:[-+].*)?$
	semanticRegexp := regexp.MustCompile(`^([0-9]+)\.([0-9]+)\.([0-9]+)(?:[-+].*)?$`)
	matches = semanticRegexp.FindStringSubmatch(version)
	if len(matches) >= 4 {
		major, err := strconv.Atoi(matches[1])
		if err != nil {
			return 0, false, fmt.Errorf("failed to parse major version from %s: %v", version, err)
		}
		minor, err := strconv.Atoi(matches[2])
		if err != nil {
			return 0, false, fmt.Errorf("failed to parse minor version from %s: %v", version, err)
		}
		patch, err := strconv.Atoi(matches[3])
		if err != nil {
			return 0, false, fmt.Errorf("failed to parse patch version from %s: %v", version, err)
		}

		// Encode semantic version: semanticVersionBase + major*1_000_000 + minor*1_000 + patch
		encoded := semanticVersionBase + major*1_000_000 + minor*1_000 + patch
		return encoded, true, nil
	}

	return 0, false, fmt.Errorf("malformed TFE version %s", version)
}

// parseTFEVersion parses TFE version strings and returns the numeric representation.
// This function maintains backward compatibility for existing callers.
// 
// Deprecated: New code should use parseTFEVersionDetailed to distinguish between
// legacy and semantic version formats.
func parseTFEVersion(version string) (int, error) {
	versionNum, _, err := parseTFEVersionDetailed(version)
	return versionNum, err
}
