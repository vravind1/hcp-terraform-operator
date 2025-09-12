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

const (
	// SEMVER_BASE is the base value for semantic version encoding.
	// All semantic versions will be encoded as values >= SEMVER_BASE to ensure
	// they are always greater than legacy version values and trigger the new algorithm.
	SEMVER_BASE = 300000000
)

// parseTFEVersion parses TFE version strings supporting both legacy date-based
// versions (e.g., v202409-1) and semantic versions (e.g., 1.0.0, 1.1.2).
//
// Legacy versions are parsed as YYYYMM + revision digit (e.g., v202409-1 -> 2024091).
// Semantic versions are encoded as SEMVER_BASE + major*1000000 + minor*1000 + patch
// to ensure they are always greater than any legacy version and use the new algorithm.
func parseTFEVersion(version string) (int, error) {
	// First try to match legacy pattern ^v(\d{6})-(\d)$
	legacyRegexp := regexp.MustCompile(`^v([0-9]{6})-([0-9]{1})$`)
	matches := legacyRegexp.FindStringSubmatch(version)
	if len(matches) == 3 {
		return strconv.Atoi(matches[1] + matches[2])
	}

	// Try to parse as semantic version: major.minor.patch with optional pre-release/build metadata
	semverRegexp := regexp.MustCompile(`^(\d+)\.(\d+)\.(\d+)(?:[-+].*)?$`)
	matches = semverRegexp.FindStringSubmatch(version)
	if len(matches) >= 4 {
		major, err1 := strconv.Atoi(matches[1])
		minor, err2 := strconv.Atoi(matches[2])
		patch, err3 := strconv.Atoi(matches[3])

		if err1 != nil || err2 != nil || err3 != nil {
			return 0, fmt.Errorf("malformed TFE version %s", version)
		}

		// Encode semantic version to ensure it's always > legacy threshold
		encoded := SEMVER_BASE + major*1000000 + minor*1000 + patch
		return encoded, nil
	}

	return 0, fmt.Errorf("malformed TFE version %s", version)
}
