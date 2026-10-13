package kvm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

const vim4HelperPath = "/usr/libexec/vim4-jetkvm"

// vim4Helper runs one bounded root operation. The application stays khadas;
// sudoers allows only this helper, which validates every request itself.
// Requests travel on stdin so credentials never appear in process arguments.
func vim4Helper(timeout time.Duration, operation string, request any, response any) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if request == nil {
		request = struct{}{}
	}
	input, err := json.Marshal(request)
	if err != nil {
		return err
	}
	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, "sudo", "-n", vim4HelperPath, operation)
	cmd.Stdin = bytes.NewReader(input)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return fmt.Errorf("%s timed out", operation)
		}
		if message := strings.TrimSpace(stderr.String()); message != "" {
			return errors.New(message)
		}
		return fmt.Errorf("%s failed: %w", operation, err)
	}
	if response == nil {
		return nil
	}
	return json.Unmarshal(stdout.Bytes(), response)
}
