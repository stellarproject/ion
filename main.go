package main

import (
	"encoding/json"
	"fmt"
	"net"
	"strings"

	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/cni/pkg/types"
	"github.com/containernetworking/cni/pkg/types/current"
	cniversion "github.com/containernetworking/cni/pkg/version"
	"github.com/gomodule/redigo/redis"
	"github.com/stellarproject/ion/version"
	"github.com/stellarproject/orbit/store"
)

const (
	ipsKey = "stellarproject.io/ips"
)

type Net struct {
	Name       string      `json:"name"`
	CNIVersion string      `json:"cniVersion"`
	IPAM       *IPAMConfig `json:"ipam"`
}

type IPAMConfig struct {
	Type        string `json:"type"`
	SubnetRange string `json:"subnet_range"`
	Gateway     string `json:"gateway"`
}

type subnetRange struct {
	Start  net.IP
	End    net.IP
	Subnet *net.IPNet
}

func main() {
	skel.PluginMain(cmdAdd, cmdGet, cmdDel, cniversion.All, version.Version)
}

func cmdGet(args *skel.CmdArgs) error {
	return fmt.Errorf("not implemented")
}

func loadConfig(bytes []byte, envArgs string) (*IPAMConfig, string, error) {
	n := Net{}
	if err := json.Unmarshal(bytes, &n); err != nil {
		return nil, "", err
	}

	if n.IPAM == nil {
		return nil, "", fmt.Errorf("config missing 'ipam' key")
	}

	if n.IPAM.SubnetRange == "" {
		return nil, "", fmt.Errorf("IPAM config missing 'subnet_range' key")
	}

	if n.IPAM.Gateway == "" {
		return nil, "", fmt.Errorf("IPAM config missing 'gateway' key")
	}

	return n.IPAM, n.CNIVersion, nil
}

func cmdAdd(args *skel.CmdArgs) error {
	cfg, confVersion, err := loadConfig(args.StdinData, args.Args)
	if err != nil {
		return err
	}

	id := args.ContainerID
	ip, subnet, err := getOrAllocateIP(id, cfg.SubnetRange)
	if err != nil {
		return err
	}

	gw := net.ParseIP(cfg.Gateway)

	result := &current.Result{}
	result.IPs = []*current.IPConfig{
		{
			Version: "4",
			Address: net.IPNet{IP: ip, Mask: subnet.Mask},
			Gateway: gw,
		},
	}
	result.Routes = []*types.Route{
		{
			Dst: net.IPNet{IP: net.IP{0, 0, 0, 0}, Mask: net.IPv4Mask(0, 0, 0, 0)},
			GW:  gw,
		},
	}

	return types.PrintResult(result, confVersion)
}

func cmdDel(args *skel.CmdArgs) error {
	return releaseIP(args.ContainerID)
}

func getConn() (redis.Conn, error) {
	s, err := store.Client()
	if err != nil {
		return nil, err
	}
	return s.Conn(true), nil
}

func getIPs() (map[string]net.IP, error) {
	c, err := getConn()
	if err != nil {
		return nil, err
	}
	defer c.Close()

	values, err := redis.StringMap(c.Do("HGETALL", ipsKey))
	if err != nil {
		return nil, err
	}

	ips := make(map[string]net.IP, len(values))
	for id, val := range values {
		ip := net.ParseIP(string(val))
		ips[id] = ip
	}

	return ips, nil
}

func getIP(id string) (net.IP, error) {
	allIPs, err := getIPs()
	if err != nil {
		return nil, err
	}
	if ip, exists := allIPs[id]; exists {
		return ip, nil
	}
	return nil, nil
}

func getOrAllocateIP(id, subnet string) (net.IP, *net.IPNet, error) {
	r, err := parseSubnetRange(subnet)
	if err != nil {
		return nil, nil, err
	}
	ip, err := getIP(id)
	if err != nil {
		return nil, nil, err
	}
	if ip != nil {
		return ip, r.Subnet, nil
	}
	ip, err = allocateIP(id, r)
	if err != nil {
		return nil, nil, err
	}
	return ip, r.Subnet, nil
}

func allocateIP(id string, r *subnetRange) (net.IP, error) {
	c, err := getConn()
	if err != nil {
		return nil, err
	}
	defer c.Close()

	reservedIPs, err := getIPs()
	if err != nil {
		return nil, err
	}

	if ip, exists := reservedIPs[id]; exists {
		return ip, nil
	}

	lookup := map[string]string{}
	for id, ip := range reservedIPs {
		lookup[ip.String()] = id
	}

	for ip := r.Start; !ip.Equal(r.End); nextIP(ip) {
		// filter out network, gateway and broadcast
		if !validIP(ip) {
			continue
		}
		if _, exists := lookup[ip.String()]; exists {
			// ip already reserved
			continue
		}
		// save
		if _, err := c.Do("HSET", ipsKey, id, ip.String()); err != nil {
			return nil, err
		}

		return ip, nil
	}

	return nil, fmt.Errorf("no available IPs")
}

func releaseIP(id string) error {
	c, err := getConn()
	if err != nil {
		return err
	}
	defer c.Close()

	ip, err := getIP(id)
	if err != nil {
		return err
	}

	if ip != nil {
		if _, err := c.Do("HDEL", ipsKey, id); err != nil {
			return err
		}
	}
	return nil
}

func nextIP(ip net.IP) {
	for j := len(ip) - 1; j >= 0; j-- {
		ip[j]++
		if ip[j] > 0 {
			break
		}
	}
}

func validIP(ip net.IP) bool {
	v := ip[len(ip)-1]
	switch v {
	case 0, 1, 255:
		return false
	}
	return true
}

// parseSubnetRange parses the subnet range
// format can either be a subnet like 10.0.0.0/8 or range like 10.0.0.100-10.0.0.200/24
func parseSubnetRange(subnet string) (*subnetRange, error) {
	parts := strings.Split(subnet, "-")
	if len(parts) == 1 {
		ip, sub, err := net.ParseCIDR(parts[0])
		if err != nil {
			return nil, err
		}
		end := make(net.IP, len(ip))
		copy(end, ip)
		end[len(end)-1] = 254
		return &subnetRange{
			Start:  ip,
			End:    end,
			Subnet: sub,
		}, nil
	}
	if len(parts) > 2 || !strings.Contains(subnet, "/") {
		return nil, fmt.Errorf("invalid range specified; expect format 10.0.0.100-10.0.0.200/24")
	}
	s := net.ParseIP(parts[0])
	e, sub, err := net.ParseCIDR(parts[1])
	if err != nil {
		return nil, err
	}
	return &subnetRange{
		Start:  s,
		End:    e,
		Subnet: sub,
	}, nil
}
