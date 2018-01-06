package cni

import (
	"errors"
	"fmt"
	"net"
	"os/exec"
	"regexp"
	"runtime"

	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/cni/pkg/types/current"
	"github.com/containernetworking/cni/pkg/version"
	"github.com/kelda/kelda/minion/ipdef"
	"github.com/kelda/kelda/minion/network/openflow"
	"github.com/kelda/kelda/minion/nl"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

const mtu int = 1400

// execRun is a variable so that it can be mocked out by the unit tests.
var execRun = func(name string, args ...string) error {
	return exec.Command(name, args...).Run()
}

var addFlows = openflow.AddFlows

func cmdDel(args *skel.CmdArgs) error {
	podName, err := grepPodName(args.Args)
	if err != nil {
		return err
	}

	ip, _, err := getIPMac(podName)
	if err != nil {
		return err
	}

	ipStr := ip.IP.String()
	outerName := ipdef.IFName(ipStr)
	peerBr, peerKelda := ipdef.PatchPorts(ipStr)
	err = execRun("ovs-vsctl",
		"--", "del-port", ipdef.KeldaBridge, outerName,
		"--", "del-port", ipdef.KeldaBridge, peerKelda,
		"--", "del-port", ipdef.OvnBridge, peerBr)
	if err != nil {
		return fmt.Errorf("failed to teardown OVS ports: %s", err)
	}

	link, err := nl.N.LinkByName(outerName)
	if err != nil {
		return fmt.Errorf("failed to find outer veth: %s", err)
	}

	if err := nl.N.LinkDel(link); err != nil {
		return fmt.Errorf("failed to delete veth %s: %s", link.Attrs().Name, err)
	}

	return nil
}

func cmdAdd(args *skel.CmdArgs) error {
	result, err := cmdAddResult(args)
	if err != nil {
		return err
	}
	return result.Print()
}

func cmdAddResult(args *skel.CmdArgs) (current.Result, error) {
	podName, err := grepPodName(args.Args)
	if err != nil {
		return current.Result{}, err
	}

	ip, mac, err := getIPMac(podName)
	if err != nil {
		return current.Result{}, err
	}

	outerName := ipdef.IFName(ip.IP.String())
	tmpPodName := ipdef.IFName("-" + outerName)
	if err := nl.N.AddVeth(outerName, tmpPodName, mtu); err != nil {
		return current.Result{},
			fmt.Errorf("failed to create veth %s: %s", outerName, err)
	}

	if err := setupPod(args.Netns, args.IfName, tmpPodName, ip, mac); err != nil {
		return current.Result{}, err
	}

	if err := setupOuterLink(outerName); err != nil {
		return current.Result{}, err
	}

	if err := setupOVS(outerName, ip.IP, mac); err != nil {
		return current.Result{}, fmt.Errorf("failed to setup OVS: %s", err)
	}

	iface := current.Interface{Name: "eth0", Mac: mac.String(), Sandbox: args.Netns}
	ipconfig := current.IPConfig{
		Version:   "4",
		Interface: current.Int(0),
		Address:   ip,
		Gateway:   net.IPv4(10, 0, 0, 1),
	}

	result := current.Result{
		CNIVersion: "0.3.1",
		Interfaces: []*current.Interface{&iface},
		IPs:        []*current.IPConfig{&ipconfig},
	}
	return result, nil
}

func grepPodName(args string) (string, error) {
	nameRegex := regexp.MustCompile("K8S_POD_NAME=([^;]+);")
	sm := nameRegex.FindStringSubmatch(args)
	if len(sm) < 2 {
		return "", errors.New("failed to find pod name in arguments")
	}
	return sm[1], nil
}

func setupOuterLink(name string) error {
	link, err := nl.N.LinkByName(name)
	if err != nil {
		return fmt.Errorf("failed to find link %s: %s", name, err)
	}

	if err := nl.N.LinkSetUp(link); err != nil {
		return fmt.Errorf("failed to bring link up: %s", err)
	}

	return nil
}

func setupPod(ns, goalName, vethName string, ip net.IPNet, mac net.HardwareAddr) error {
	// This function jumps into the pod namespace and thus can't risk being
	// scheduled onto an OS thread that hasn't made the jump.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	link, err := nl.N.LinkByName(vethName)
	if err != nil {
		return fmt.Errorf("failed to find link %s: %s", vethName, err)
	}

	rootns, err := nl.N.GetNetns()
	if err != nil {
		return fmt.Errorf("failed to get current namespace handle: %s", err)
	}

	nsh, err := nl.N.GetNetnsFromPath(ns)
	if err != nil {
		return fmt.Errorf("failed to open network namespace handle: %s", err)
	}
	defer nl.N.CloseNsHandle(nsh)

	if err := nl.N.LinkSetNs(link, nsh); err != nil {
		return fmt.Errorf("failed to put link in pod namespace: %s", err)
	}

	if err := nl.N.SetNetns(nsh); err != nil {
		return fmt.Errorf("failed to enter pod network namespace: %s", err)
	}
	defer nl.N.SetNetns(rootns)

	if err := nl.N.LinkSetHardwareAddr(link, mac); err != nil {
		return fmt.Errorf("failed to set mac address: %s", err)
	}

	if err := nl.N.AddrAdd(link, ip); err != nil {
		return fmt.Errorf("failed to set IP %s: %s", ip.String(), err)
	}

	if err := nl.N.LinkSetName(link, goalName); err != nil {
		return fmt.Errorf("failed to set device name: %s", err)
	}

	if err := nl.N.LinkSetUp(link); err != nil {
		return fmt.Errorf("failed to bring link up: %s", err)
	}

	podIndex := link.Attrs().Index
	err = nl.N.RouteAdd(nl.Route{
		Scope:     nl.ScopeLink,
		LinkIndex: podIndex,
		Dst:       &ipdef.KeldaSubnet,
		Src:       ip.IP,
	})
	if err != nil {
		return fmt.Errorf("failed to add route: %s", err)
	}

	err = nl.N.RouteAdd(nl.Route{LinkIndex: podIndex, Gw: ipdef.GatewayIP})
	if err != nil {
		return fmt.Errorf("failed to add default route: %s", err)
	}

	return nil
}

var getPodLabels = func(podName string) (map[string]string, error) {
	kubeconfig, err := clientcmd.BuildConfigFromFlags("", "/var/lib/kubelet/kubeconfig")
	if err != nil {
		return nil, fmt.Errorf("parse kubeconfig: %s", err)
	}

	clientset, err := kubernetes.NewForConfig(kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("get kube client: %s", err)
	}

	podsClient := clientset.CoreV1().Pods(corev1.NamespaceDefault)
	pod, err := podsClient.Get(podName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("get pod: %s", err)
	}

	return pod.Labels, nil
}

func getIPMac(podName string) (net.IPNet, net.HardwareAddr, error) {
	labels, err := getPodLabels(podName)
	if err != nil {
		return net.IPNet{}, nil, err
	}

	ipStr, ok := labels["keldaIP"]
	if !ok {
		return net.IPNet{}, nil, errors.New("pod has no Kelda IP")
	}

	ip := net.ParseIP(ipStr)
	if ip == nil {
		return net.IPNet{}, nil, fmt.Errorf("invalid IP: %s", ipStr)
	}

	macStr := ipdef.IPToMac(ip)
	mac, err := net.ParseMAC(macStr)
	if err != nil {
		err := fmt.Errorf("failed to parse mac address %s: %s", macStr, err)
		return net.IPNet{}, nil, err
	}

	return net.IPNet{IP: ip, Mask: net.IPv4Mask(255, 255, 255, 255)}, mac, nil
}

func setupOVS(outerName string, ip net.IP, mac net.HardwareAddr) error {
	peerBr, peerKelda := ipdef.PatchPorts(ip.String())
	err := execRun("ovs-vsctl",
		"--", "add-port", ipdef.KeldaBridge, outerName,

		"--", "add-port", ipdef.KeldaBridge, peerKelda,

		"--", "set", "Interface", peerKelda, "type=patch",
		"options:peer="+peerBr,

		"--", "add-port", ipdef.OvnBridge, peerBr,

		"--", "set", "Interface", peerBr, "type=patch",
		"options:peer="+peerKelda,
		"external-ids:attached-mac="+mac.String(),
		"external-ids:iface-id="+ip.String())
	if err != nil {
		return fmt.Errorf("failed to configure OVSDB: %s", err)
	}

	err = addFlows([]openflow.Container{{
		Veth:  outerName,
		Patch: peerKelda,
		Mac:   mac.String(),
		IP:    ip.String(),
	}})
	if err != nil {
		return fmt.Errorf("failed to populate OpenFlow tables: %s", err)
	}

	return nil
}

// Main ... TODO Ethan: Document this.
func Main() {
	skel.PluginMain(cmdAdd, cmdDel, version.PluginSupports(version.Current()))
}
