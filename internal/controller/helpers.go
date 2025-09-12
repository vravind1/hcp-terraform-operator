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
	// Threshold at which the “new” autoscaling algorithm becomes available for legacy versions.
	// Encoding (minimal Option A): legacy numeric = YYYYMM + buildNumber (concatenated).
	legacyVersionThreshold = 2024091   // v202409-1 -> 2024091
	semanticVersionBase    = 300000000 // all semantic encodings start at or above this value
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

// formatOutput converts a Terraform Cloud/Enterprise state output value to string form.
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

func needToAddFinalizer[T Object](o T, finalizer string) bool {
	return o.GetDeletionTimestamp().IsZero() && !controllerutil.ContainsFinalizer(o, finalizer)
}

func isDeletionCandidate[T Object](o T, finalizer string) bool {
	return !o.GetDeletionTimestamp().IsZero() && controllerutil.ContainsFinalizer(o, finalizer)
}

func configMapKeyRef(ctx context.Context, c client.Client, nn types.NamespacedName, key string) (string, error) {
	cm := &corev1.ConfigMap{}
	if err := c.Get(ctx, nn, cm); err != nil {
		return "", err
	}
	if v, ok := cm.Data[key]; ok {
		return v, nil
	}
	return "", fmt.Errorf("unable to find key=%q in configMap=%q namespace=%q", key, nn.Name, nn.Namespace)
}

func secretKeyRef(ctx context.Context, c client.Client, nn types.NamespacedName, key string) (string, error) {
	secret := &corev1.Secret{}
	if err := c.Get(ctx, nn, secret); err != nil {
		return "", err
	}
	if v, ok := secret.Data[key]; ok {
		return strings.TrimSpace(string(v)), nil
	}
	return "", fmt.Errorf("unable to find key=%q in secret=%q namespace=%q", key, nn.Name, nn.Namespace)
}

// parseTFEVersionDetailed parses exactly two supported formats:
//
// 1. Legacy date-based: vYYYYMM-N
//    - Regex: ^v([0-9]{6})-([0-9]+)$
//    - Encoding: concatenate YYYYMM + build (e.g. v202409-10 -> 20240910)
//    - Returns isSemantic=false.
//
// 2. Strict semantic: MAJOR.MINOR.PATCH
//    - Regex: ^([0-9]+)\.([0-9]+)\.([0-9]+)$
//    - NO prerelease or build metadata accepted.
//    - Encoding: semanticVersionBase + major*1_000_000 + minor*1_000 + patch
//    - Returns isSemantic=true.
//
// Returns: (numericEncoding, isSemantic, error)
func parseTFEVersionDetailed(version string) (int, bool, error) {
	legacyRe := regexp.MustCompile(`^v([0-9]{6})-([0-9]+)$`)
	if m := legacyRe.FindStringSubmatch(version); len(m) == 3 {
		num, err := strconv.Atoi(m[1] + m[2])
		if err != nil {
			return 0, false, fmt.Errorf("failed to parse legacy TFE version %q: %w", version, err)
		}
		return num, false, nil
	}

	semanticRe := regexp.MustCompile(`^([0-9]+)\.([0-9]+)\.([0-9]+)$`)
	if m := semanticRe.FindStringSubmatch(version); len(m) == 4 {
		major, err := strconv.Atoi(m[1])
		if err != nil {
			return 0, false, fmt.Errorf("failed to parse major in %q: %w", version, err)
		}
		minor, err := strconv.Atoi(m[2])
		if err != nil {
			return 0, false, fmt.Errorf("failed to parse minor in %q: %w", version, err)
		}
		patch, err := strconv.Atoi(m[3])
		if err != nil {
			return 0, false, fmt.Errorf("failed to parse patch in %q: %w", version, err)
		}
		encoded := semanticVersionBase + major*1_000_000 + minor*1_000 + patch
		return encoded, true, nil
	}

	return 0, false, fmt.Errorf("malformed TFE version %q (supported: vYYYYMM-N or MAJOR.MINOR.PATCH)", version)
}

// parseTFEVersion returns only the numeric encoding (legacy compatibility).
// Deprecated: prefer parseTFEVersionDetailed for new code.
func parseTFEVersion(version string) (int, error) {
	v, _, err := parseTFEVersionDetailed(version)
	return v, err
}