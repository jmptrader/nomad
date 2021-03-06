package driver

import (
	"fmt"
	"io/ioutil"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/hashicorp/nomad/client/config"
	"github.com/hashicorp/nomad/client/driver/environment"
	"github.com/hashicorp/nomad/nomad/structs"
)

func TestRawExecDriver_Fingerprint(t *testing.T) {
	d := NewRawExecDriver(testDriverContext(""))
	node := &structs.Node{
		Attributes: make(map[string]string),
	}

	// Disable raw exec.
	cfg := &config.Config{Options: map[string]string{rawExecConfigOption: "false"}}

	apply, err := d.Fingerprint(cfg, node)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if apply {
		t.Fatalf("should not apply")
	}
	if node.Attributes["driver.raw_exec"] != "" {
		t.Fatalf("driver incorrectly enabled")
	}

	// Enable raw exec.
	cfg.Options[rawExecConfigOption] = "true"
	apply, err = d.Fingerprint(cfg, node)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !apply {
		t.Fatalf("should apply")
	}
	if node.Attributes["driver.raw_exec"] != "1" {
		t.Fatalf("driver not enabled")
	}
}

func TestRawExecDriver_StartOpen_Wait(t *testing.T) {
	task := &structs.Task{
		Name: "sleep",
		Config: map[string]string{
			"command": "/bin/sleep",
			"args":    "1",
		},
	}
	driverCtx := testDriverContext(task.Name)
	ctx := testDriverExecContext(task, driverCtx)
	defer ctx.AllocDir.Destroy()

	d := NewRawExecDriver(driverCtx)
	handle, err := d.Start(ctx, task)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if handle == nil {
		t.Fatalf("missing handle")
	}

	// Attempt to open
	handle2, err := d.Open(ctx, handle.ID())
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if handle2 == nil {
		t.Fatalf("missing handle")
	}

	// Task should terminate quickly
	select {
	case <-handle2.WaitCh():
	case <-time.After(2 * time.Second):
		t.Fatalf("timeout")
	}

	// Check they are both tracking the same PID.
	pid1 := handle.(*rawExecHandle).proc.Pid
	pid2 := handle2.(*rawExecHandle).proc.Pid
	if pid1 != pid2 {
		t.Fatalf("tracking incorrect Pid; %v != %v", pid1, pid2)
	}
}

func TestRawExecDriver_Start_Wait(t *testing.T) {
	task := &structs.Task{
		Name: "sleep",
		Config: map[string]string{
			"command": "/bin/sleep",
			"args":    "1",
		},
	}

	driverCtx := testDriverContext(task.Name)
	ctx := testDriverExecContext(task, driverCtx)
	defer ctx.AllocDir.Destroy()

	d := NewRawExecDriver(driverCtx)
	handle, err := d.Start(ctx, task)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if handle == nil {
		t.Fatalf("missing handle")
	}

	// Update should be a no-op
	err = handle.Update(task)
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	// Task should terminate quickly
	select {
	case err := <-handle.WaitCh():
		if err != nil {
			t.Fatalf("err: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timeout")
	}
}

func TestRawExecDriver_Start_Wait_AllocDir(t *testing.T) {
	exp := []byte{'w', 'i', 'n'}
	file := "output.txt"
	task := &structs.Task{
		Name: "sleep",
		Config: map[string]string{
			"command": "/bin/bash",
			"args":    fmt.Sprintf(`-c "sleep 1; echo -n %s > $%s/%s"`, string(exp), environment.AllocDir, file),
		},
	}

	driverCtx := testDriverContext(task.Name)
	ctx := testDriverExecContext(task, driverCtx)
	defer ctx.AllocDir.Destroy()

	d := NewRawExecDriver(driverCtx)
	handle, err := d.Start(ctx, task)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if handle == nil {
		t.Fatalf("missing handle")
	}

	// Task should terminate quickly
	select {
	case err := <-handle.WaitCh():
		if err != nil {
			t.Fatalf("err: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timeout")
	}

	// Check that data was written to the shared alloc directory.
	outputFile := filepath.Join(ctx.AllocDir.SharedDir, file)
	act, err := ioutil.ReadFile(outputFile)
	if err != nil {
		t.Fatalf("Couldn't read expected output: %v", err)
	}

	if !reflect.DeepEqual(act, exp) {
		t.Fatalf("Command outputted %v; want %v", act, exp)
	}
}

func TestRawExecDriver_Start_Kill_Wait(t *testing.T) {
	task := &structs.Task{
		Name: "sleep",
		Config: map[string]string{
			"command": "/bin/sleep",
			"args":    "1",
		},
	}

	driverCtx := testDriverContext(task.Name)
	ctx := testDriverExecContext(task, driverCtx)
	defer ctx.AllocDir.Destroy()

	d := NewRawExecDriver(driverCtx)
	handle, err := d.Start(ctx, task)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if handle == nil {
		t.Fatalf("missing handle")
	}

	go func() {
		time.Sleep(100 * time.Millisecond)
		err := handle.Kill()
		if err != nil {
			t.Fatalf("err: %v", err)
		}
	}()

	// Task should terminate quickly
	select {
	case err := <-handle.WaitCh():
		if err == nil {
			t.Fatal("should err")
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timeout")
	}
}
