package geospatial

import (
	"strings"
	"testing"
)

func validRequest() OperationRequest {
	return OperationRequest{
		APIVersion: APIVersionV1Alpha1,
		Kind:       RequestKind,
		Operation:  OperationValidate,
		Inputs: map[string]InputSpec{
			"field_raster": {Path: `/worker/cache/yanroy/tile_001/fields.tif`, Band: intPtr(1), Nodata: intPtr(0)},
			"value_raster": {Path: `C:\worker\cache\cdl\2023\cdl.tif`, Band: intPtr(1), Nodata: intPtr(0)},
		},
		Outputs: map[string]string{
			"counts_csv":    "field_crop_counts.csv",
			"metadata_json": "metadata/field_crop_counts.json",
		},
		Options: map[string]any{
			"require_aligned_grid": true,
		},
	}
}

func TestOperationRequestValidateAcceptsWorkerLocalInputPaths(t *testing.T) {
	request := validRequest()
	if err := request.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestOperationRequestValidateRejectsInvalidEnvelopeAndOperation(t *testing.T) {
	cases := []struct {
		name string
		edit func(*OperationRequest)
		want string
	}{
		{name: "api version", edit: func(r *OperationRequest) { r.APIVersion = "goet.geospatial/v9" }, want: "unsupported api_version"},
		{name: "kind", edit: func(r *OperationRequest) { r.Kind = "Other" }, want: "unsupported kind"},
		{name: "missing operation", edit: func(r *OperationRequest) { r.Operation = "" }, want: "unsupported operation"},
		{name: "unsupported operation", edit: func(r *OperationRequest) { r.Operation = "raster_pair_value_counts" }, want: "unsupported operation"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			request := validRequest()
			tc.edit(&request)
			err := request.Validate()
			if err == nil {
				t.Fatal("expected an error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Validate() error = %v, want substring %q", err, tc.want)
			}
		})
	}
}

func TestOperationRequestValidateRequiresMetadataOutputForRasterInfo(t *testing.T) {
	request := validRequest()
	request.Operation = OperationRasterInfo
	request.Outputs = map[string]string{}
	if err := request.Validate(); err == nil {
		t.Fatal("expected error for missing metadata_json output")
	}
}

func TestOperationRequestValidateRequiresInputsForRasterInfo(t *testing.T) {
	request := validRequest()
	request.Operation = OperationRasterInfo
	request.Inputs = map[string]InputSpec{}
	if err := request.Validate(); err == nil {
		t.Fatal("expected error for raster_info with no inputs")
	}
}

func TestOperationRequestValidateRejectsUnsafeOutputPaths(t *testing.T) {
	cases := []struct {
		name string
		path string
	}{
		{name: "empty", path: ""},
		{name: "absolute", path: "/outputs/counts.csv"},
		{name: "windows drive", path: "C:/outputs/counts.csv"},
		{name: "backslash", path: `outputs\counts.csv`},
		{name: "parent traversal", path: "../counts.csv"},
		{name: "nested parent traversal", path: "outputs/../counts.csv"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			request := validRequest()
			request.Outputs = map[string]string{"counts_csv": tc.path}
			err := request.Validate()
			if err == nil {
				t.Fatal("expected an error")
			}
			if !strings.Contains(err.Error(), "output \"counts_csv\" path") {
				t.Fatalf("Validate() error = %v, want output path context", err)
			}
		})
	}
}

func intPtr(value int) *int {
	return &value
}
