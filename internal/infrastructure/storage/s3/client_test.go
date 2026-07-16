package s3

import "testing"

func TestNewClient_ValidEndpoint_Succeeds(t *testing.T) {
	c, err := NewClient("http://localhost:9000", "us-east-1", "my-bucket", "access", "secret", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c == nil {
		t.Fatal("expected a non-nil client")
	}
}

func TestNewClient_HTTPSEndpoint_Succeeds(t *testing.T) {
	c, err := NewClient("https://r2.example.com", "auto", "my-bucket", "access", "secret", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c == nil {
		t.Fatal("expected a non-nil client")
	}
}

func TestNewClient_MalformedEndpoint_ReturnsError(t *testing.T) {
	if _, err := NewClient("://not-a-valid-url", "us-east-1", "bucket", "a", "s", false); err == nil {
		t.Fatal("expected an error for a malformed endpoint URL")
	}
}

func TestNewClient_EndpointWithoutHost_ReturnsError(t *testing.T) {
	if _, err := NewClient("just-a-path", "us-east-1", "bucket", "a", "s", false); err == nil {
		t.Fatal("expected an error when the endpoint has no host component")
	}
}

func TestKeyFromURL_PathStyle(t *testing.T) {
	c := &Client{bucket: "my-bucket"}

	key, err := c.keyFromURL("http://localhost:9000/my-bucket/reports/2026/result-1.pdf")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if key != "reports/2026/result-1.pdf" {
		t.Errorf("expected key %q, got %q", "reports/2026/result-1.pdf", key)
	}
}

func TestKeyFromURL_VirtualHostedStyle(t *testing.T) {
	c := &Client{bucket: "my-bucket"}

	key, err := c.keyFromURL("https://my-bucket.s3.amazonaws.com/reports/result-2.pdf")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if key != "reports/result-2.pdf" {
		t.Errorf("expected key %q, got %q", "reports/result-2.pdf", key)
	}
}

func TestKeyFromURL_EmptyKey_ReturnsError(t *testing.T) {
	c := &Client{bucket: "my-bucket"}

	if _, err := c.keyFromURL("http://localhost:9000/my-bucket/"); err == nil {
		t.Fatal("expected an error for a URL with an empty object key")
	}
}

func TestKeyFromURL_MalformedURL_ReturnsError(t *testing.T) {
	c := &Client{bucket: "my-bucket"}

	if _, err := c.keyFromURL("://not-a-valid-url"); err == nil {
		t.Fatal("expected an error for a malformed object URL")
	}
}

func TestKeyFromURL_RoundTripsWithUploadURLShape(t *testing.T) {
	// Upload() returns endpointURL + "/" + bucket + "/" + key — keyFromURL
	// must invert that exact shape for DeleteByURL/PresignedGetURL to work.
	c := &Client{bucket: "my-bucket"}
	uploadedURL := "http://localhost:9000/my-bucket/deep/nested/key with spaces.pdf"

	key, err := c.keyFromURL(uploadedURL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if key != "deep/nested/key with spaces.pdf" {
		t.Errorf("expected key %q, got %q", "deep/nested/key with spaces.pdf", key)
	}
}
