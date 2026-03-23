package network

import (
	"net"
)

type Interface struct {
	Name string
	IP   string
}

func GetInterfaces() ([]Interface, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	var results []Interface
	for _, i := range ifaces {
		addrs, err := i.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip != nil && ip.To4() != nil && !ip.IsLoopback() {
				results = append(results, Interface{
					Name: i.Name,
					IP:   ip.String(),
				})
			}
		}
	}
	return results, nil
}
