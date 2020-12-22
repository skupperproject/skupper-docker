package main

import (
	"github.com/skupperproject/skupper-docker/api/types"
)

func getBridgeName(address string, host string) string {
	if host == "" {
		return address
	} else {
		return address + "@" + host
	}
}

// selector ~ type one of container, host-service
type EgressBindings struct {
	name       string
	selector   string
	service    string
	egressPort int
}

type ServiceBindings struct {
	origin       string
	protocol     string
	address      string
	publicPort   int
	ingressPort  int
	aggregation  string
	eventChannel bool
	headless     *types.Headless
	targets      map[string]*EgressBindings
}

func asServiceInterface(bindings *ServiceBindings) types.ServiceInterface {
	si := types.ServiceInterface{
		Address:      bindings.address,
		Protocol:     bindings.protocol,
		Port:         bindings.publicPort,
		Aggregate:    bindings.aggregation,
		EventChannel: bindings.eventChannel,
		Headless:     bindings.headless,
		Origin:       bindings.origin,
	}
	for _, eb := range bindings.targets {
		si.Targets = append(si.Targets, types.ServiceInterfaceTarget{
			Name:       eb.name,
			Selector:   eb.selector,
			TargetPort: eb.egressPort,
			Service:    eb.service,
		})
	}
	return si
}

func getTargetPort(service types.ServiceInterface, target types.ServiceInterfaceTarget) int {
	targetPort := target.TargetPort
	if targetPort == 0 {
		targetPort = service.Port
	}
	return targetPort
}

func hasTargetForName(si types.ServiceInterface, name string) bool {
	for _, t := range si.Targets {
		if si.Address+"@"+t.Name == name {
			return true
		}
	}
	return false
}

func hasTargetForSelector(si types.ServiceInterface, selector string) bool {
	for _, t := range si.Targets {
		if t.Selector == selector {
			return true
		}
	}
	return false
}

func hasTargetForService(si types.ServiceInterface, service string) bool {
	for _, t := range si.Targets {
		if t.Service == service {
			return true
		}
	}
	return false
}

func (c *Controller) updateServiceBindings(required types.ServiceInterface) error {
	bindings := c.bindings[required.Address]
	if bindings == nil {
		sb := newServiceBindings(required.Origin, required.Protocol, required.Address, required.Port, required.Headless, required.Port, required.Aggregate, required.EventChannel)
		for _, t := range required.Targets {
			sb.targets[required.Address+"@"+t.Name] = &EgressBindings{
				name:       t.Name,
				selector:   t.Selector,
				service:    t.Service,
				egressPort: t.TargetPort,
			}
		}
		c.bindings[required.Address] = sb
	} else {
		//check it is configured correctly
		if bindings.protocol != required.Protocol {
			bindings.protocol = required.Protocol
		}
		if bindings.publicPort != required.Port {
			bindings.publicPort = required.Port
		}
		if bindings.aggregation != required.Aggregate {
			bindings.aggregation = required.Aggregate
		}
		if bindings.eventChannel != required.EventChannel {
			bindings.eventChannel = required.EventChannel
		}
		if bindings.origin != required.Origin {
			bindings.origin = required.Origin
		}

		for _, t := range required.Targets {
			targetPort := getTargetPort(required, t)
			target := bindings.targets[required.Address+"@"+t.Name]
			if target == nil {
				bindings.addTarget(required.Address, t.Name, t.Selector, targetPort, c)
			} else if target.egressPort != targetPort {
				target.egressPort = targetPort
			}
		}

		for k, _ := range bindings.targets {
			if !hasTargetForName(required, k) {
				delete(bindings.targets, k)
			}
		}
	}
	return nil
}

func newServiceBindings(origin string, protocol string, address string, publicPort int, headless *types.Headless, ingressPort int, aggregation string, eventChannel bool) *ServiceBindings {
	return &ServiceBindings{
		origin:       origin,
		protocol:     protocol,
		address:      address,
		publicPort:   publicPort,
		ingressPort:  ingressPort,
		aggregation:  aggregation,
		eventChannel: eventChannel,
		headless:     headless,
		targets:      map[string]*EgressBindings{},
	}
}

func (sb *ServiceBindings) addTarget(address string, name string, selector string, port int, controller *Controller) error {
	sb.targets[address+"@"+name] = &EgressBindings{
		name:       name,
		selector:   selector,
		egressPort: port,
	}
	return nil
}

func (sb *ServiceBindings) addSelectorTarget(name string, selector string, port int, controller *Controller) error {
	sb.targets[selector] = &EgressBindings{
		name:       name,
		selector:   selector,
		egressPort: port,
	}
	return nil
}

func (sb *ServiceBindings) removeSelectorTarget(selector string) {
	delete(sb.targets, selector)
}

func (sb *ServiceBindings) addServiceTarget(name string, service string, port int, controller *Controller) error {
	sb.targets[service] = &EgressBindings{
		name:       name,
		service:    service,
		egressPort: port,
	}
	return nil
}

func (sb *ServiceBindings) removeServiceTarget(service string) {
	delete(sb.targets, service)
}
