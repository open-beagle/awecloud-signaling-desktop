//go:build windows

package rdp

import (
	"encoding/binary"
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const (
	afInet                 = 2
	afInet6                = 23
	tcpTableBasicListener  = 0
	errorInsufficientBuf   = 122
	mibTCPStateListenValue = 2
)

var (
	modIPHlpAPI             = windows.NewLazySystemDLL("iphlpapi.dll")
	procGetExtendedTcpTable = modIPHlpAPI.NewProc("GetExtendedTcpTable")
)

func probePlatformLocalRDP() LocalRDPStatus {
	status := LocalRDPStatus{
		Supported: true,
		LocalPort: DefaultRDPPort,
		OS:        "windows",
	}

	denyConnections, port := readWindowsRDPRegistry()
	status.LocalPort = port
	status.ServiceEnabled = !denyConnections
	status.PortListening = isWindowsTCPPortListening(port)
	status.Ready = status.ServiceEnabled && status.PortListening

	switch {
	case status.Ready:
		status.Reason = fmt.Sprintf("Windows 远程桌面已就绪（本地监听端口 %d）", status.LocalPort)
	case !status.ServiceEnabled:
		status.Reason = "系统尚未启用远程桌面（可在 Windows 设置 -> 系统 -> 远程桌面 中开启）"
	default:
		status.Reason = fmt.Sprintf("系统已启用远程桌面配置，但本地端口 %d 未处于监听状态", status.LocalPort)
	}
	return status
}

func readWindowsRDPRegistry() (denyConnections bool, port int) {
	denyConnections = true
	port = DefaultRDPPort

	tsKey, err := registry.OpenKey(registry.LOCAL_MACHINE, `SYSTEM\CurrentControlSet\Control\Terminal Server`, registry.QUERY_VALUE)
	if err == nil {
		if val, _, valErr := tsKey.GetIntegerValue("fDenyTSConnections"); valErr == nil {
			denyConnections = (val != 0)
		}
		_ = tsKey.Close()
	}

	wsKey, err := registry.OpenKey(registry.LOCAL_MACHINE, `SYSTEM\CurrentControlSet\Control\Terminal Server\WinStations\RDP-Tcp`, registry.QUERY_VALUE)
	if err == nil {
		if val, _, valErr := wsKey.GetIntegerValue("PortNumber"); valErr == nil && val > 0 && val <= 65535 {
			port = int(val)
		}
		_ = wsKey.Close()
	}

	return denyConnections, port
}

// isWindowsTCPPortListening 通过 iphlpapi.dll GetExtendedTcpTable 读取内核监听表，不发起任何本地 TCP 连接握手
func isWindowsTCPPortListening(targetPort int) bool {
	if targetPort <= 0 || targetPort > 65535 {
		return false
	}
	return checkWindowsTCPListenerTable(afInet, targetPort) || checkWindowsTCPListenerTable(afInet6, targetPort)
}

func checkWindowsTCPListenerTable(family uintptr, targetPort int) bool {
	if err := procGetExtendedTcpTable.Find(); err != nil {
		return false
	}

	var size uint32
	r1, _, _ := procGetExtendedTcpTable.Call(
		0,
		uintptr(unsafe.Pointer(&size)),
		0,
		family,
		tcpTableBasicListener,
		0,
	)
	if r1 != errorInsufficientBuf || size == 0 {
		return false
	}

	buf := make([]byte, size)
	r1, _, _ = procGetExtendedTcpTable.Call(
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(unsafe.Pointer(&size)),
		0,
		family,
		tcpTableBasicListener,
		0,
	)
	if r1 != 0 || len(buf) < 4 {
		return false
	}

	numEntries := int(binary.LittleEndian.Uint32(buf[0:4]))
	offset := 4

	if family == afInet {
		// MIB_TCPROW: dwState(4), dwLocalAddr(4), dwLocalPort(4), dwRemoteAddr(4), dwRemotePort(4) = 20 bytes
		const rowSize = 20
		for i := 0; i < numEntries && offset+rowSize <= len(buf); i++ {
			state := binary.LittleEndian.Uint32(buf[offset : offset+4])
			rawLocalPort := binary.LittleEndian.Uint32(buf[offset+8 : offset+12])
			if state == mibTCPStateListenValue && parseWin32NetworkPort(rawLocalPort) == targetPort {
				return true
			}
			offset += rowSize
		}
		return false
	}

	// MIB_TCP6ROW: State(4), LocalAddr(16), LocalScopeId(4), LocalPort(4), RemoteAddr(16), RemoteScopeId(4), RemotePort(4) = 52 bytes
	const row6Size = 52
	for i := 0; i < numEntries && offset+row6Size <= len(buf); i++ {
		state := binary.LittleEndian.Uint32(buf[offset : offset+4])
		rawLocalPort := binary.LittleEndian.Uint32(buf[offset+24 : offset+28])
		if state == mibTCPStateListenValue && parseWin32NetworkPort(rawLocalPort) == targetPort {
			return true
		}
		offset += row6Size
	}
	return false
}
