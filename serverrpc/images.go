package serverrpc

import (
	"bytes"
	"io"
	"io/ioutil"

	"fmt"
	enginetypes "github.com/docker/engine-api/types"
	"github.com/golang/glog"
	"github.com/hyperhq/hyperd/types"
	"golang.org/x/net/context"
)

// ImageList implements GET /images/get
func (s *ServerRPC) ImageList(ctx context.Context, req *types.ImageListRequest) (*types.ImageListResponse, error) {
	images, err := s.daemon.Daemon.Images(req.FilterArgs, req.Filter, req.All)
	if err != nil {
		return nil, err
	}

	result := make([]*types.ImageInfo, 0, len(images))
	for _, image := range images {
		result = append(result, &types.ImageInfo{
			Id:          image.ID,
			ParentID:    image.ParentID,
			RepoTags:    image.RepoTags,
			RepoDigests: image.RepoDigests,
			Created:     image.Created,
			VirtualSize: image.VirtualSize,
			Labels:      image.Labels,
		})
	}

	return &types.ImageListResponse{
		ImageList: result,
	}, nil
}

// ImagePull pulls a image from registry
func (s *ServerRPC) ImagePull(req *types.ImagePullRequest, stream types.PublicAPI_ImagePullServer) error {
	authConfig := &enginetypes.AuthConfig{}
	if req.Auth != nil {
		authConfig = &enginetypes.AuthConfig{
			Username:      req.Auth.Username,
			Password:      req.Auth.Password,
			Auth:          req.Auth.Auth,
			Email:         req.Auth.Email,
			ServerAddress: req.Auth.Serveraddress,
			RegistryToken: req.Auth.Registrytoken,
		}
	}
	glog.V(3).Infof("ImagePull with ServerStream %s request %s", stream, req.String())

	r, w := io.Pipe()

	var pullResult error
	var complete = false

	go func() {
		defer r.Close()
		for {
			data := make([]byte, 512)
			n, err := r.Read(data)
			if err == io.EOF {
				if complete {
					break
				} else {
					continue
				}
			}

			if err != nil {
				glog.Errorf("Read image pull stream error: %v", err)
				return
			}

			if err := stream.Send(&types.ImagePullResponse{Data: data[:n]}); err != nil {
				glog.Errorf("Send image pull progress to stream error: %v", err)
				return
			}
		}
	}()

	pullResult = s.daemon.CmdImagePull(req.Image, req.Tag, authConfig, nil, w)
	complete = true

	if pullResult != nil {
		pullResult = fmt.Errorf("s.daemon.CmdImagePull with request %s error: %v", req.String(), pullResult)
	}
	return pullResult
}

// ImagePush pushes a local image to registry
func (s *ServerRPC) ImagePush(req *types.ImagePushRequest, stream types.PublicAPI_ImagePushServer) error {
	authConfig := &enginetypes.AuthConfig{}
	if req.Auth != nil {
		authConfig = &enginetypes.AuthConfig{
			Username:      req.Auth.Username,
			Password:      req.Auth.Password,
			Auth:          req.Auth.Auth,
			Email:         req.Auth.Email,
			ServerAddress: req.Auth.Serveraddress,
			RegistryToken: req.Auth.Registrytoken,
		}
	}
	glog.V(3).Infof("ImagePush with ServerStream %s request %s", stream, req.String())

	buffer := bytes.NewBuffer([]byte{})
	var pushResult error
	var complete = false
	go func() {
		pushResult = s.daemon.CmdImagePush(req.Repo, req.Tag, authConfig, nil, buffer)
		complete = true
	}()

	for {
		data, err := ioutil.ReadAll(buffer)
		if err == io.EOF {
			if complete {
				break
			} else {
				continue
			}
		}

		if err != nil {
			return fmt.Errorf("ImagePush read image push stream with request %s error: %v", req.String(), err)
		}

		if err := stream.Send(&types.ImagePushResponse{Data: data}); err != nil {
			return fmt.Errorf("stream.Send with request %s error: %v", req.String(), err)
		}
	}

	if pushResult != nil {
		pushResult = fmt.Errorf("s.daemon.CmdImagePush with request %s error: %v", req.String(), pushResult)
	}
	return pushResult
}

// ImageRemove deletes a image from hyperd
func (s *ServerRPC) ImageRemove(ctx context.Context, req *types.ImageRemoveRequest) (*types.ImageRemoveResponse, error) {
	resp, err := s.daemon.CmdImageDelete(req.Image, req.Force, req.Prune)
	if err != nil {
		return nil, err
	}

	return &types.ImageRemoveResponse{
		Images: resp,
	}, nil
}
