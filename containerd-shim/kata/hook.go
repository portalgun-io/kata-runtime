// Copyright (c) 2018 Intel Corporation
// Copyright (c) 2018 HyperHQ Inc.
//
// SPDX-License-Identifier: Apache-2.0
//

package kata

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	sysexec "os/exec"
	"syscall"
	"time"

	"github.com/kata-containers/runtime/virtcontainers/pkg/oci"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/sirupsen/logrus"
)

func runHook(ctx context.Context, hook specs.Hook, cid, bundlePath string) error {
	state := specs.State{
		Pid:    os.Getpid(),
		Bundle: bundlePath,
		ID:     cid,
	}

	stateJSON, err := json.Marshal(state)
	if err != nil {
		return err
	}

	var stdout, stderr bytes.Buffer
	cmd := &sysexec.Cmd{
		Path:   hook.Path,
		Args:   hook.Args,
		Env:    hook.Env,
		Stdin:  bytes.NewReader(stateJSON),
		Stdout: &stdout,
		Stderr: &stderr,
	}

	if err := cmd.Start(); err != nil {
		return err
	}

	if hook.Timeout == nil {
		if err := cmd.Wait(); err != nil {
			return fmt.Errorf("%s: stdout: %s, stderr: %s", err, stdout.String(), stderr.String())
		}
	} else {
		done := make(chan error, 1)
		go func() {
			done <- cmd.Wait()
			close(done)
		}()

		select {
		case err := <-done:
			if err != nil {
				return fmt.Errorf("%s: stdout: %s, stderr: %s", err, stdout.String(), stderr.String())
			}
		case <-time.After(time.Duration(*hook.Timeout) * time.Second):
			if err := syscall.Kill(cmd.Process.Pid, syscall.SIGKILL); err != nil {
				return err
			}

			return fmt.Errorf("Hook timeout")
		}
	}

	return nil
}

func runHooks(ctx context.Context, hooks []specs.Hook, cid, bundlePath, hookType string) error {
	for _, hook := range hooks {
		if err := runHook(ctx, hook, cid, bundlePath); err != nil {
			logrus.WithFields(logrus.Fields{
				"hook-type": hookType,
				"error":     err,
			}).Error("hook error")

			return err
		}
	}

	return nil
}

func preStartHooks(ctx context.Context, spec oci.CompatOCISpec, cid, bundlePath string) error {
	// If no hook available, nothing needs to be done.
	if spec.Hooks == nil {
		return nil
	}

	return runHooks(ctx, spec.Hooks.Prestart, cid, bundlePath, "pre-start")
}

func postStartHooks(ctx context.Context, spec oci.CompatOCISpec, cid, bundlePath string) error {
	// If no hook available, nothing needs to be done.
	if spec.Hooks == nil {
		return nil
	}

	return runHooks(ctx, spec.Hooks.Poststart, cid, bundlePath, "post-start")
}

func postStopHooks(ctx context.Context, spec oci.CompatOCISpec, cid, bundlePath string) error {
	// If no hook available, nothing needs to be done.
	if spec.Hooks == nil {
		return nil
	}

	return runHooks(ctx, spec.Hooks.Poststop, cid, bundlePath, "post-stop")
}