package chat

import "testing"

func TestCollectCandidateMediaPrefersImageDataURL(t *testing.T) {
	dataURL := "data:image/jpeg;base64,/9j/2Q=="

	items := collectCandidateMedia([]Message{
		{
			ID:   "msg-1",
			Kind: "IMAGE",
			NormalizedPayload: map[string]interface{}{
				"image_data_url":  dataURL,
				"image_url":       "https://files.example.test/doc.jpg",
				"image_mime_type": "image/jpeg",
			},
		},
	})

	if len(items) != 1 {
		t.Fatalf("expected one media item, got %d", len(items))
	}
	if items[0].URL != dataURL {
		t.Fatalf("unexpected media url: %s", items[0].URL)
	}
	if items[0].MimeType != "image/jpeg" {
		t.Fatalf("unexpected mime type: %s", items[0].MimeType)
	}
	if items[0].MessageID != "msg-1" {
		t.Fatalf("unexpected message id: %s", items[0].MessageID)
	}
}

func TestCollectCandidateMediaFallsBackToImageURL(t *testing.T) {
	items := collectCandidateMedia([]Message{
		{
			ID:   "msg-1",
			Kind: "IMAGE",
			NormalizedPayload: map[string]interface{}{
				"image_url":       "https://files.example.test/doc.jpg",
				"image_mime_type": "image/jpeg",
			},
		},
	})

	if len(items) != 1 {
		t.Fatalf("expected one media item, got %d", len(items))
	}
	if items[0].URL != "https://files.example.test/doc.jpg" {
		t.Fatalf("unexpected media url: %s", items[0].URL)
	}
}

func TestCollectCandidateMediaIncludesPDFDataURLAndFileName(t *testing.T) {
	dataURL := "data:application/pdf;base64,JVBERi0xLjQ="

	items := collectCandidateMedia([]Message{
		{
			ID:   "msg-1",
			Kind: "DOCUMENT",
			NormalizedPayload: map[string]interface{}{
				"document_data_url":  dataURL,
				"document_url":       "https://files.example.test/rg.pdf",
				"document_mime_type": "application/pdf",
				"document_file_name": "rg-frente-verso.pdf",
			},
		},
	})

	if len(items) != 1 {
		t.Fatalf("expected one media item, got %d", len(items))
	}
	if items[0].Kind != "PDF" {
		t.Fatalf("expected PDF kind, got %s", items[0].Kind)
	}
	if items[0].URL != dataURL {
		t.Fatalf("expected PDF data URL, got %s", items[0].URL)
	}
	if items[0].FileName != "rg-frente-verso.pdf" {
		t.Fatalf("expected file name to be preserved, got %s", items[0].FileName)
	}
}
