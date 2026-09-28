package app

import (
	"context"
	"encoding/json"
	"time"

	"github.com/perfect-panel/server/internal/auth/usersession"
	"github.com/perfect-panel/server/internal/transport/devicesocket"
	"github.com/perfect-panel/server/pkg/logger"
)

// NewDeviceManager builds the device WebSocket manager. The devices'
// presence is the identity module's: the online and offline callbacks record
// it through the identity facade, resolved when a callback runs because the
// manager is built before the facade. The socket has no one to report a
// failed record to, so it is only logged.
func NewDeviceManager(srv *Application) *devicesocket.DeviceManager {
	// The socket callbacks run on the connections' goroutines, outside any
	// request, so their writes use a root context.
	ctx := context.Background()
	manager := devicesocket.NewDeviceManager(30, 30)

	manager.OnDeviceOffline = func(userID int64, deviceID, session string, createAt time.Time) {
		if err := srv.Identity.MarkDeviceOffline(ctx, userID, deviceID, createAt); err != nil {
			logger.Errorw("[DeviceManager] record device offline failed", logger.Field("error", err.Error()), logger.Field("device_id", deviceID))
		}
	}

	manager.OnDeviceOnline = func(userID int64, deviceID, session string) {
		if err := srv.Identity.MarkDeviceOnline(ctx, deviceID); err != nil {
			logger.Errorw("[DeviceManager] record device online failed", logger.Field("error", err.Error()), logger.Field("device_id", deviceID))
		}
	}

	manager.OnDeviceKicked = func(userID int64, deviceID, session string, operator devicesocket.Operator) {
		var message DeviceMessage
		switch operator {
		case devicesocket.Admin:
			// An administrator kicked the device.
			message = DeviceMessage{Method: DeviceKickedAdmin}
		case devicesocket.MaxDevices:
			// The user signed in on more devices than the limit.
			message = DeviceMessage{Method: DeviceKickedMax}
		default:
			return
		}
		_ = manager.SendToDevice(userID, deviceID, message.Json())
		// The kicked session ends; the user's other sessions stay.
		if err := usersession.End(ctx, srv.Redis, session); err != nil {
			logger.Errorw("[DeviceManager] end kicked session failed", logger.Field("error", err.Error()), logger.Field("device_id", deviceID))
		}
	}

	manager.OnMessage = func(userID int64, deviceID, session string, message string) {
		logger.Infof("userid: %d ,device_number: %s,session: %s, message: %v", userID, deviceID, session, message)
	}
	return manager
}

// DeviceMessage is a message the server pushes to a connected device.
type DeviceMessage struct {
	Method DeviceMessageMethod `json:"method"`
}

// Json encodes the message as the device protocol's JSON text.
func (dm *DeviceMessage) Json() string {
	jsonData, _ := json.Marshal(dm)
	return string(jsonData)
}

// DeviceMessageMethod names what a device message tells the device.
type DeviceMessageMethod string

const (
	// DeviceKickedMax tells a device it was signed out because its user
	// signed in on more devices than the limit allows.
	DeviceKickedMax DeviceMessageMethod = "kicked_device"
	// DeviceKickedAdmin tells a device an administrator signed it out.
	DeviceKickedAdmin DeviceMessageMethod = "kicked_admin"
	// SubscribeUpdate tells a device its subscription changed.
	SubscribeUpdate DeviceMessageMethod = "subscribe_update"
)
