package containerd

import (
	"context"
	"time"

	api "github.com/containerd/containerd/api/services/images/v1"
	"github.com/containerd/containerd/api/types"
	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
)

// ImageService is the image service interface
type ImageService interface {
	Create(ctx context.Context, image Image) (Image, error)
	Update(ctx context.Context, image Image, fieldpaths ...string) (Image, error)
}

type imageService struct {
	client api.ImagesClient
}

// Create creates a new image
func (s *imageService) Create(ctx context.Context, image Image) (Image, error) {
	req := &api.CreateImageRequest{
		Image: &api.Image{
			Name: image.Name,
			Target: &types.Descriptor{
				MediaType: image.Target.MediaType,
				Digest:    image.Target.Digest.String(),
				Size:      image.Target.Size,
			},
		},
	}

	resp, err := s.client.Create(ctx, req)
	if err != nil {
		return Image{}, err
	}

	return imageFromProto(resp.Image), nil
}

// Update updates an existing image
func (s *imageService) Update(ctx context.Context, image Image, fieldpaths ...string) (Image, error) {
	req := &api.UpdateImageRequest{
		Image: &api.Image{
			Name: image.Name,
			Target: &types.Descriptor{
				MediaType: image.Target.MediaType,
				Digest:    image.Target.Digest.String(),
				Size:      image.Target.Size,
			},
		},
	}

	// If no fieldpaths specified, update all fields
	if len(fieldpaths) == 0 {
		req.UpdateMask = &fieldmaskpb.FieldMask{
			Paths: []string{"target"},
		}
	} else {
		req.UpdateMask = &fieldmaskpb.FieldMask{
			Paths: fieldpaths,
		}
	}

	resp, err := s.client.Update(ctx, req)
	if err != nil {
		return Image{}, err
	}

	return imageFromProto(resp.Image), nil
}

// Image represents a container image
type Image struct {
	Name      string
	Labels    map[string]string
	Target    ocispec.Descriptor
	CreatedAt time.Time
	UpdatedAt time.Time
}

func imageFromProto(proto *api.Image) Image {
	return Image{
		Name:   proto.Name,
		Labels: proto.Labels,
		Target: ocispec.Descriptor{
			MediaType: proto.Target.MediaType,
			Digest:    digest.Digest(proto.Target.Digest),
			Size:      proto.Target.Size,
		},
		CreatedAt: proto.CreatedAt.AsTime(),
		UpdatedAt: proto.UpdatedAt.AsTime(),
	}
}
