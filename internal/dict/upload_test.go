package dict

import "testing"

func TestUploadOptionsUseEnumOrder(t *testing.T) {
	drivers := UploadDriverOptions()
	if len(drivers) != 2 || drivers[0].Value != "cos" || drivers[1].Value != "oss" {
		t.Fatalf("unexpected upload driver options: %#v", drivers)
	}

	imageExts := UploadImageExtOptions()
	if len(imageExts) == 0 || imageExts[0].Value != "jpeg" {
		t.Fatalf("unexpected image extension options: %#v", imageExts)
	}

	fileExts := UploadFileExtOptions()
	if len(fileExts) == 0 || fileExts[0].Value != "docx" {
		t.Fatalf("unexpected file extension options: %#v", fileExts)
	}
}
