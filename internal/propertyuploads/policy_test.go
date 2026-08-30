package propertyuploads

import "testing"

func TestDecideUpload(t *testing.T) {
	tests := []struct {
		name    string
		request UploadRequest
		wantKey string
		wantMax int64
		wantErr bool
	}{
		{"maintenance photo", UploadRequest{"elm-court", "req-104", MaintenanceRequest, "image/jpeg"}, "properties/elm-court/maintenance_request/req-104", 15 << 20, false},
		{"tenant PDF", UploadRequest{"elm-court", "lease-8", TenantDocument, "application/pdf"}, "properties/elm-court/tenant_document/lease-8", 10 << 20, false},
		{"inspection PDF", UploadRequest{"elm-court", "inspection-2", InspectionReminder, "application/pdf"}, "properties/elm-court/inspection_reminder/inspection-2", 8 << 20, false},
		{"reject executable tenant file", UploadRequest{"elm-court", "lease-9", TenantDocument, "application/octet-stream"}, "", 0, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := DecideUpload(tc.request)
			if (err != nil) != tc.wantErr {
				t.Fatalf("DecideUpload() error = %v, wantErr %v", err, tc.wantErr)
			}
			if got.Key != tc.wantKey || got.MaxBytes != tc.wantMax {
				t.Fatalf("DecideUpload() = key %q, max %d; want key %q, max %d", got.Key, got.MaxBytes, tc.wantKey, tc.wantMax)
			}
		})
	}
}
