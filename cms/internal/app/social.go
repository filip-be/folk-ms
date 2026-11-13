package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/filip-be/folk-ms/cms/internal/domain"
	"github.com/filip-be/folk-ms/cms/internal/ports"
)

// MediaCacheControl is the Cache-Control of content-hashed media.
const MediaCacheControl = "public, max-age=31536000, immutable"

// UploadImage processes an upload into JPEG variants under immutable,
// content-hashed names in the public store.
func (a *App) UploadImage(ctx context.Context, p domain.Principal, org string, kind ports.ImageKind, data []byte) (*domain.Image, error) {
	if _, err := a.authorize(p, org); err != nil {
		return nil, err
	}
	if kind != ports.ImageCover && kind != ports.ImageSocial {
		return nil, &domain.ValidationError{Problems: []domain.FieldError{{Field: "kind", Message: domain.Text{PL: "nieznany rodzaj obrazu", EN: "unknown image kind"}}}}
	}
	img, err := a.storeImage(ctx, org, kind, data)
	if err != nil {
		return nil, err
	}
	a.audit(ctx, p, "image.uploaded", org, img.ID, map[string]any{"kind": kind, "bytes": len(data)})
	return img, nil
}

// storeImage processes an image into JPEG variants under immutable,
// content-hashed names in the public store. Re-encoding also strips
// anything an uploaded or downloaded file carried besides pixels.
func (a *App) storeImage(ctx context.Context, org string, kind ports.ImageKind, data []byte) (*domain.Image, error) {
	rends, err := a.Images.Process(data, kind)
	if err != nil {
		// Adapters describe a rejected upload with Localized; other errors
		// are shown as they are.
		var ue interface{ Localized() (pl, en string) }
		if errors.As(err, &ue) {
			pl, en := ue.Localized()
			return nil, domain.Problem("image", pl, en)
		}
		return nil, domain.Problem("image", err.Error(), err.Error())
	}
	sum := sha256.Sum256(append([]byte(kind+"\x00"), data...))
	hash := hex.EncodeToString(sum[:8])
	img := &domain.Image{ID: hash}
	for _, r := range rends {
		path := fmt.Sprintf("media/%s/%s-%s.jpg", org, hash, r.Name)
		if _, err := a.Pub.Put(ctx, path, r.Data, "image/jpeg", MediaCacheControl, ""); err != nil && !errors.Is(err, ports.ErrPrecondition) {
			return nil, err // ErrPrecondition: the same upload exists already
		}
		v := domain.ImageVariant{Width: r.Width, Height: r.Height, Path: path}
		if r.Name == "ig" {
			img.Instagram = &v
		} else {
			img.Variants = append(img.Variants, v)
		}
	}
	return img, nil
}

