package kvm

import "fmt"

// Optional because the appliance backend has no independent output scaler.
type videoOutputScaler interface {
	VideoSetOutputHeight(int) error
}

func rpcGetVideoOutputHeight() (int, error) {
	if _, ok := nativeInstance.(videoOutputScaler); !ok {
		return 0, fmt.Errorf("output scaling is unavailable on this device")
	}
	return config.VideoOutputHeight, nil
}

func rpcSetVideoOutputHeight(height int) error {
	scaler, ok := nativeInstance.(videoOutputScaler)
	if !ok {
		return fmt.Errorf("output scaling is unavailable on this device")
	}
	if err := scaler.VideoSetOutputHeight(height); err != nil {
		return err
	}
	previous := config.VideoOutputHeight
	config.VideoOutputHeight = height
	if err := SaveConfig(); err != nil {
		config.VideoOutputHeight = previous
		_ = scaler.VideoSetOutputHeight(previous)
		return fmt.Errorf("failed to save output height: %w", err)
	}
	return nil
}
