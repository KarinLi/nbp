package iscsi

import (
	"errors"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/mitchellh/mapstructure"
)

// IscsiConnectorInfo define
type IscsiConnectorInfo struct {
	AccessMode string `mapstructure:"accessMode"`
	AuthUser   string `mapstructure:"authUserName"`
	AuthPass   string `mapstructure:"authPassword"`
	AuthMethod string `mapstructure:"authMethod"`
	TgtDisco   bool   `mapstructure:"targetDiscovered"`
	TgtIQN     string `mapstructure:"targetIqn"`
	TgtPortal  string `mapstructure:"targetPortal"`
	VolumeID   string `mapstructure:"volumeId"`
	TgtLun     int    `mapstructure:"targetLun"`
	Encrypted  bool   `mapstructure:"encrypted"`
}

////////////////////////////////////////////////////////////////////////////////
//      Refer some codes from: https://github.com/j-griffith/csi-cinder       //
//      Refer some codes from: https://github.com/kubernetes/kubernetes       //
////////////////////////////////////////////////////////////////////////////////

const (
	//ISCSITranslateTCP tcp
	ISCSITranslateTCP = "tcp"
)

// statFunc define
type statFunc func(string) (os.FileInfo, error)

// globFunc define
type globFunc func(string) ([]string, error)

// waitForPathToExist scan the device path
func waitForPathToExist(devicePath *string, maxRetries int, deviceTransport string) bool {
	// This makes unit testing a lot easier
	return waitForPathToExistInternal(devicePath, maxRetries, deviceTransport, os.Stat, filepath.Glob)
}

// waitForPathToExistInternal scan the device path
func waitForPathToExistInternal(devicePath *string, maxRetries int, deviceTransport string, osStat statFunc, filepathGlob globFunc) bool {
	if devicePath == nil {
		return false
	}

	for i := 0; i < maxRetries; i++ {
		var err error
		if deviceTransport == ISCSITranslateTCP {
			_, err = osStat(*devicePath)
		} else {
			fpath, _ := filepathGlob(*devicePath)
			if fpath == nil {
				err = os.ErrNotExist
			} else {
				// There might be a case that fpath contains multiple device paths if
				// multiple PCI devices connect to same iscsi target. We handle this
				// case at subsequent logic. Pick up only first path here.
				*devicePath = fpath[0]
			}
		}
		if err == nil {
			return true
		}
		if !os.IsNotExist(err) {
			return false
		}
		if i == maxRetries-1 {
			break
		}
		time.Sleep(time.Second)
	}
	return false
}

func execCmd(name string, arg ...string) (string, error) {
	log.Printf("Command: %s %s\n", name, strings.Join(arg, " "))
	info, err := exec.Command(name, arg...).CombinedOutput()
	return string(info), err
}

// GetInitiator returns all the ISCSI Initiator Name
func GetInitiator() ([]string, error) {
	res, err := execCmd("cat", "/etc/iscsi/initiatorname.iscsi")
	log.Printf("result from cat: %s", res)
	iqns := []string{}
	if err != nil {
		log.Printf("Error encountered gathering initiator names: %v", err)
		return iqns, nil
	}

	lines := strings.Split(string(res), "\n")
	for _, l := range lines {
		log.Printf("Inspect line: %s", l)
		if strings.Contains(l, "InitiatorName=") {
			iqns = append(iqns, strings.Split(l, "=")[1])
		}
	}

	log.Printf("Found the following iqns: %s", iqns)
	return iqns, nil
}

// Discovery ISCSI Target
func Discovery(portal string) error {
	log.Printf("Discovery portal: %s", portal)
	_, err := execCmd("iscsiadm", "-m", "discovery", "-t", "sendtargets", "-p", portal)
	if err != nil {
		log.Fatalf("Error encountered in sendtargets: %v", err)
		return err
	}
	return nil
}

// Login ISCSI Target
func SetAuth(portal string, targetiqn string, name string, passwd string) error {
	log.Println("Set user auth", portal, targetiqn, name, passwd)
	// Set UserName
	info, err := execCmd("iscsiadm", "-m", "node", "-p", portal, "-T", targetiqn,
		"--op=update", "--name", "node.session.auth.username", "--value", name)
	if err != nil {
		log.Fatalf("Received error on set income username: %v, %v", err, info)
		return err
	}
	// Set Password
	info, err = execCmd("iscsiadm", "-m", "node", "-p", portal, "-T", targetiqn,
		"--op=update", "--name", "node.session.auth.password", "--value", passwd)
	if err != nil {
		log.Fatalf("Received error on set income password: %v, %v", err, info)
		return err
	}
	return nil
}

// Login ISCSI Target
func Login(portal string, targetiqn string) error {
	log.Printf("Login portal: %s targetiqn: %s", portal, targetiqn)
	info, err := execCmd("iscsiadm", "-m", "node", "-p", portal, "-T", targetiqn, "--login")
	if err != nil {
		log.Fatalln("Received error on login attempt", err, info)
		return err
	}
	return nil
}

// Logout ISCSI Target
func Logout(portal string, targetiqn string) error {
	log.Printf("Logout portal: %s targetiqn: %s", portal, targetiqn)
	info, err := execCmd("iscsiadm", "-m", "node", "-p", portal, "-T", targetiqn, "--logout")
	if err != nil {
		log.Fatalln("Received error on logout attempt", err, info)
		return err
	}
	return nil
}

// Delete ISCSI Node
func Delete(targetiqn string) (err error) {
	log.Printf("Delete targetiqn: %s", targetiqn)
	_, err = execCmd("iscsiadm", "-m", "node", "-o", "delete", "-T", targetiqn)
	if err != nil {
		log.Fatalf("Received error on Delete attempt: %v", err)
		return err
	}
	return nil
}

// Connect ISCSI Target
func Connect(connMap map[string]interface{}) (string, error) {
	conn := ParseIscsiConnectInfo(connMap)
	log.Println(connMap)
	log.Println(conn)
	portal := conn.TgtPortal
	targetiqn := conn.TgtIQN
	targetlun := strconv.Itoa(conn.TgtLun)

	log.Printf("Connect portal: %s targetiqn: %s targetlun: %s", portal, targetiqn, targetlun)
	devicePath := strings.Join([]string{
		"/dev/disk/by-path/ip",
		portal,
		"iscsi",
		targetiqn,
		"lun",
		targetlun}, "-")

	isexist := waitForPathToExist(&devicePath, 1, ISCSITranslateTCP)
	if !isexist {

		// Discovery
		err := Discovery(portal)
		if err != nil {
			return "", err
		}
		if len(conn.AuthMethod) != 0 {
			SetAuth(portal, targetiqn, conn.AuthUser, conn.AuthPass)
		}
		//Login
		err = Login(portal, targetiqn)
		if err != nil {
			return "", err
		}

		isexist = waitForPathToExist(&devicePath, 10, ISCSITranslateTCP)

		if !isexist {
			return "", errors.New("Could not connect volume: Timeout after 10s")
		}

	}
	return devicePath, nil
}

// Disconnect ISCSI Target
func Disconnect(portal string, targetiqn string) error {
	log.Printf("Disconnect portal: %s targetiqn: %s", portal, targetiqn)

	// Logout
	err := Logout(portal, targetiqn)
	if err != nil {
		return err
	}

	//Delete
	err = Delete(targetiqn)
	if err != nil {
		return err
	}

	return nil
}

// GetFSType returns the File System Type of device
func GetFSType(device string) string {
	log.Printf("GetFSType: %s", device)
	fsType := ""
	res, err := execCmd("blkid", device)
	if err != nil {
		log.Printf("failed to GetFSType: %v", err)
		return fsType
	}

	if strings.Contains(string(res), "TYPE=") {
		for _, v := range strings.Split(string(res), " ") {
			if strings.Contains(v, "TYPE=") {
				fsType = strings.Split(v, "=")[1]
				fsType = strings.Replace(fsType, "\"", "", -1)
			}
		}
	}
	return fsType
}

// Format device by File System Type
func Format(device string, fstype string) error {
	log.Printf("Format device: %s fstype: %s", device, fstype)

	// Get current File System Type
	curFSType := GetFSType(device)
	if curFSType == "" {
		// Default File Sysem Type is ext4
		if fstype == "" {
			fstype = "ext4"
		}
		_, err := execCmd("mkfs", "-t", fstype, "-F", device)
		if err != nil {
			log.Fatalf("failed to Format: %v", err)
			return err
		}
	} else {
		log.Printf("Device: %s has been formatted yet. fsType: %s", device, curFSType)
	}
	return nil
}

// Mount device into mount point
func Mount(device string, mountpoint string) error {
	log.Printf("Mount device: %s mountpoint: %s", device, mountpoint)

	_, err := execCmd("mkdir", "-p", mountpoint)
	if err != nil {
		log.Fatalf("failed to mkdir: %v", err)
	}
	_, err = execCmd("mount", device, mountpoint)
	if err != nil {
		log.Fatalf("failed to mount: %v", err)
		return err
	}
	return nil
}

// FormatAndMount device
func FormatAndMount(device string, fstype string, mountpoint string) error {
	log.Printf("FormatAndMount device: %s fstype: %s mountpoint: %s", device, fstype, mountpoint)

	// Format
	err := Format(device, fstype)
	if err != nil {
		return err
	}

	// Mount
	err = Mount(device, mountpoint)
	if err != nil {
		return err
	}

	return nil
}

// Umount from mountpoint
func Umount(mountpoint string) error {
	log.Printf("Umount mountpoint: %s", mountpoint)

	_, err := execCmd("umount", mountpoint)
	if err != nil {
		log.Fatalf("failed to Umount: %v", err)
		return err
	}
	return nil
}

// ParseIscsiConnectInfo decode
func ParseIscsiConnectInfo(connectInfo map[string]interface{}) *IscsiConnectorInfo {
	var con IscsiConnectorInfo
	mapstructure.Decode(connectInfo, &con)
	return &con
}

// GetHostIp return Host IP
func GetHostIp() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "127.0.0.1"
	}

	for _, address := range addrs {
		if ipnet, ok := address.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			return ipnet.IP.String()
		}
	}

	return "127.0.0.1"
}
