package model

import (
	"errors"
	"strings"
	"testing"
)

func TestResolveMaterializationDomainShared(t *testing.T) {
	domain, err := ResolveMaterializationDomain(
		DataAssetMaterialization{Scope: DataMaterializationScopeShared, Strategy: DataAssetCacheStrategyWorkerCache},
		"msu-hpcc",
	)
	if err != nil {
		t.Fatalf("ResolveMaterializationDomain() error = %v", err)
	}
	if domain.Scope != DataMaterializationScopeShared || domain.ID != "msu-hpcc" {
		t.Fatalf("domain = %+v", domain)
	}
}

func TestResolveMaterializationDomainSharedRequiresDomainID(t *testing.T) {
	_, err := ResolveMaterializationDomain(
		DataAssetMaterialization{Scope: DataMaterializationScopeShared, Strategy: DataAssetCacheStrategyWorkerCache},
		"",
	)
	if err == nil || !strings.Contains(err.Error(), "shared materialization domain id is required") {
		t.Fatalf("error = %v, want missing shared domain", err)
	}
}

func TestResolveMaterializationDomainWorkerIsKnownButNotImplemented(t *testing.T) {
	_, err := ResolveMaterializationDomain(
		DataAssetMaterialization{Scope: DataMaterializationScopeWorker, Strategy: DataAssetCacheStrategyWorkerCache},
		"worker-a",
	)
	if !errors.Is(err, ErrMaterializationScopeNotImplemented) {
		t.Fatalf("error = %v, want ErrMaterializationScopeNotImplemented", err)
	}
}

func TestResolveMaterializationDomainRejectsUnknownScope(t *testing.T) {
	_, err := ResolveMaterializationDomain(
		DataAssetMaterialization{Scope: "cluster", Strategy: DataAssetCacheStrategyWorkerCache},
		"msu-hpcc",
	)
	if err == nil {
		t.Fatal("ResolveMaterializationDomain() succeeded, want invalid scope")
	}
	if errors.Is(err, ErrMaterializationScopeNotImplemented) {
		t.Fatalf("error = %v, unknown scope must be invalid, not not-implemented", err)
	}
	if !strings.Contains(err.Error(), `unsupported materialization scope "cluster"`) {
		t.Fatalf("error = %v, want unsupported scope", err)
	}
}

func TestResolveMaterializationDomainRequiresExplicitScope(t *testing.T) {
	_, err := ResolveMaterializationDomain(
		DataAssetMaterialization{Strategy: DataAssetCacheStrategyWorkerCache},
		"msu-hpcc",
	)
	if err == nil || !strings.Contains(err.Error(), "materialization scope is required") {
		t.Fatalf("error = %v, want required scope", err)
	}
}

func TestMaterializationLookupKeyIncludesScopeAndDomain(t *testing.T) {
	assetKey := "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	first, err := MaterializationLookupKey(
		assetKey,
		DataAssetMaterialization{Scope: DataMaterializationScopeShared, Strategy: DataAssetCacheStrategyWorkerCache},
		"msu-hpcc",
	)
	if err != nil {
		t.Fatalf("MaterializationLookupKey(first) error = %v", err)
	}
	second, err := MaterializationLookupKey(
		assetKey,
		DataAssetMaterialization{Scope: DataMaterializationScopeShared, Strategy: DataAssetCacheStrategyWorkerCache},
		"target-local",
	)
	if err != nil {
		t.Fatalf("MaterializationLookupKey(second) error = %v", err)
	}
	if first == second {
		t.Fatalf("lookup keys match for different domains: %s", first)
	}
}
