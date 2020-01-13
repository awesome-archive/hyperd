package serverrpc

import (
	"encoding/json"
	"io"

	"fmt"
	"github.com/golang/glog"
	"github.com/hyperhq/hyperd/lib/promise"
	"github.com/hyperhq/hyperd/types"
	"golang.org/x/net/context"
)

func (s *ServerRPC) ExecCreate(ctx context.Context, req *types.ExecCreateRequest) (*types.ExecCreateResponse, error) {
	cmd, err := json.Marshal(req.Command)
	if err != nil {
		return nil, fmt.Errorf("json.Marshal error: %v", err)
	}

	execId, err := s.daemon.CreateExec(req.ContainerID, string(cmd), req.Tty)
	if err != nil {
		return nil, fmt.Errorf("s.daemon.CreateExec error: %v", err)
	}

	return &types.ExecCreateResponse{
		ExecID: execId,
	}, nil
}

func (s *ServerRPC) ExecStart(stream types.PublicAPI_ExecStartServer) error {
	req, err := stream.Recv()
	if err != nil {
		return fmt.Errorf("stream.Recv error: %v", err)
	}
	glog.V(3).Infof("ExecStart with ServerStream %s request %s", stream, req.String())

	inReader, inWriter := io.Pipe()
	outReader, outWriter := io.Pipe()
	outErr := promise.Go(func() (err error) {
		defer outReader.Close()
		buf := make([]byte, 32)
		for {
			nr, err := outReader.Read(buf)
			if nr > 0 {
				if err := stream.Send(&types.ExecStartResponse{buf[:nr]}); err != nil {
					return fmt.Errorf("stream.Send with request %s error: %v", req.String(), err)
				}
			}
			if err == io.EOF {
				return nil
			}
			if err != nil {
				return fmt.Errorf("outReader.Read with request %s error: %v", req.String(), err)
			}
		}
	})
	go func() {
		defer inWriter.Close()
		for {
			req, err := stream.Recv()
			if err != nil && err != io.EOF {
				glog.Errorf("Receive from stream error: %v", err)
				return
			}
			if req != nil && req.Stdin != nil {
				nw, ew := inWriter.Write(req.Stdin)
				if ew != nil {
					glog.Errorf("Write pipe error: %v", ew)
					return
				}
				if nw != len(req.Stdin) {
					glog.Errorf("Write data length is not enougt, write: %d success: %d", len(req.Stdin), nw)
					return
				}
			}
			if err == io.EOF {
				break
			}
		}
	}()

	err = s.daemon.StartExec(inReader, outWriter, req.ContainerID, req.ExecID)
	if err != nil {
		return fmt.Errorf("s.daemon.StartExec with request %s error: %v", req.String(), err)
	}
	err = <-outErr
	return err
}

// Wait gets exitcode by container and processId
func (s *ServerRPC) Wait(c context.Context, req *types.WaitRequest) (*types.WaitResponse, error) {
	//FIXME need update if param NoHang is enabled
	code, err := s.daemon.ExitCode(req.Container, req.ProcessId)
	if err != nil {
		return nil, err
	}

	return &types.WaitResponse{
		ExitCode: int32(code),
	}, nil
}

// ExecSignal sends a singal to specified exec of specified container
func (s *ServerRPC) ExecSignal(ctx context.Context, req *types.ExecSignalRequest) (*types.ExecSignalResponse, error) {
	err := s.daemon.KillExec(req.ContainerID, req.ExecID, req.Signal)
	if err != nil {
		return nil, err
	}

	return &types.ExecSignalResponse{}, nil
}

func (s *ServerRPC) ExecVM(stream types.PublicAPI_ExecVMServer) error {
	req, err := stream.Recv()
	if err != nil {
		return fmt.Errorf("stream.Recv error: %v", err)
	}
	glog.V(3).Infof("ExecVM with ServerStream %s request %s", stream, req.String())

	cmd, err := json.Marshal(req.Command)
	if err != nil {
		return fmt.Errorf("json.Marshal with request %s error: %v", req.String(), err)
	}

	inReader, inWriter := io.Pipe()
	outReader, outWriter := io.Pipe()
	go func() {
		defer outReader.Close()
		buf := make([]byte, 32)
		for {
			nr, err := outReader.Read(buf)
			if nr > 0 {
				if err := stream.Send(&types.ExecVMResponse{
					Stdout: buf[:nr],
				}); err != nil {
					glog.Errorf("Send to stream error: %v", err)
					return
				}
			}
			if err == io.EOF {
				break
			}
			if err != nil {
				glog.Errorf("Read from pipe error: %v", err)
				return
			}
		}
	}()

	go func() {
		defer inWriter.Close()
		for {
			recv, err := stream.Recv()
			if err != nil && err != io.EOF {
				glog.Errorf("Receive from stream error: %v", err)
				return
			}
			if recv != nil && recv.Stdin != nil {
				nw, ew := inWriter.Write(recv.Stdin)
				if ew != nil {
					glog.Errorf("Write pipe error: %v", ew)
					return
				}
				if nw != len(recv.Stdin) {
					glog.Errorf("Write data length is not enougt, write: %d success: %d", len(recv.Stdin), nw)
					return
				}
			}
			if err == io.EOF {
				break
			}
		}
	}()

	code, err := s.daemon.ExecVM(req.PodID, string(cmd), inReader, outWriter, outWriter)
	if err != nil {
		return fmt.Errorf("s.daemon.ExecVM with request %s error: %v", req.String(), err)
	}
	if err := stream.Send(&types.ExecVMResponse{
		ExitCode: int32(code),
	}); err != nil {
		return fmt.Errorf("stream.Send with request %s error: %v", req.String(), err)
	}

	return nil
}
