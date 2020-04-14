package main

import (
	"context"
	"encoding/json"
	"fmt"
	//	"io/ioutil"
	"log"
	"reflect"
	"sort"
	"time"

	amqp "github.com/Azure/go-amqp"

	"github.com/skupperproject/skupper-docker/api/types"
)

type ServiceSyncUpdate struct {
	origin  string
	indexed map[string]types.ServiceInterface
}

func (c *Controller) serviceSyncDefinitionsUpdated(definitions map[string]types.ServiceInterface) {
	var latest []types.ServiceInterface // becomes c.Local
	byName := make(map[string]types.ServiceInterface)
	var added []types.ServiceInterface
	var modified []types.ServiceInterface
	var removed []types.ServiceInterface

	for name, original := range definitions {
		service := types.ServiceInterface{
			Address:  original.Address,
			Protocol: original.Protocol,
			Port:     original.Port,
			Origin:   original.Origin,
			Headless: original.Headless,
			Targets:  []types.ServiceInterfaceTarget{},
		}
		if service.Origin != "" && service.Origin != "annotation" {
			if _, ok := c.byOrigin[service.Origin]; !ok {
				c.byOrigin[service.Origin] = make(map[string]types.ServiceInterface)
			}
			c.byOrigin[service.Origin][name] = service
		} else {
			latest = append(latest, service)
		}
		byName[service.Address] = service
	}

	sort.Sort(types.ByServiceInterfaceAddress(latest))

	last := make(map[string]types.ServiceInterface)
	for _, def := range c.localServices {
		last[def.Address] = def
	}
	current := make(map[string]types.ServiceInterface)
	for _, def := range latest {
		current[def.Address] = def
	}
	for _, def := range last {
		if _, ok := current[def.Address]; !ok {
			removed = append(removed, def)
		} else if !reflect.DeepEqual(def, current[def.Address]) {
			modified = append(modified, def)
		}
	}
	for _, def := range current {
		if _, ok := last[def.Address]; !ok {
			added = append(added, def)
		}
	}

	c.localServices = latest
	c.byName = byName
}

func (c *Controller) ensureServiceInterfaceDefinitions(origin string, serviceInterfaceDefs map[string]types.ServiceInterface) {
	var changed []types.ServiceInterface
	var deleted []string

	for _, def := range serviceInterfaceDefs {
		// TODO: change in origin and not equivalent service record
		// check if it already exists or exists from different origin
		//  if !ok || existing.Origin == origin && !equivalentServiceRecord(si, existing)
		if _, ok := c.byName[def.Address]; !ok {
			changed = append(changed, def)
		}

	}

	// TODO: think about aging entries
	if _, ok := c.byOrigin[origin]; !ok {
		c.byOrigin[origin] = make(map[string]types.ServiceInterface)
	} else {
		current := c.byOrigin[origin]
		for name, _ := range current {
			if _, ok := serviceInterfaceDefs[name]; !ok {
				deleted = append(deleted, name)
			}
		}
	}

	// TODO: have a func to updateAllSkupperServices
	if len(changed) > 0 || len(deleted) > 0 {
		svcDefs, err := getServices("all")
		if err != nil {
			log.Println("Failed to retrieve service definitions: ", err.Error())
			return
		}
		for _, def := range changed {
			svcDefs[def.Address] = def
		}
		for _, name := range deleted {
			delete(svcDefs, name)
		}
		err = updateServices("all", svcDefs)
		if err != nil {
			log.Println("Failed to write all service interface file: ", err.Error())
		}
	}

	return
}

func (c *Controller) syncSender(sendLocal chan bool) {
	var request amqp.Message
	var properties amqp.MessageProperties

	ctx := context.Background()
	sender, err := c.amqpSession.NewSender(amqp.LinkTargetAddress(types.ServiceSyncAddress))
	if err != nil {
		log.Fatal("Failed to create sender: ", err.Error())
	}
	defer func() {
		sender.Close(ctx)
	}()

	ticker := time.NewTicker(5 * time.Second)

	properties.Subject = "service-sync-update"
	request.Properties = &properties
	request.ApplicationProperties = make(map[string]interface{})
	request.ApplicationProperties["origin"] = c.origin

	// TODO: Have a function to getLocalSkupperServices, getAllSkupperServices
	for {
		select {
		case <-ticker.C:
			svcDefs, err := getServices("local")
			if err != nil {
				log.Println("Failed to retrieve skupper service definitions: ", err.Error())
				return
			}
			local := make([]types.ServiceInterface, 0)
			for _, si := range svcDefs {
				service := types.ServiceInterface{
					Address:  si.Address,
					Protocol: si.Protocol,
					Port:     si.Port,
					Targets:  []types.ServiceInterfaceTarget{},
				}
				//si.Targets = nil
				local = append(local, service)
			}
			encoded, err := json.Marshal(local)
			if err != nil {
				log.Println("Failed to create json for service definition sync: ", err.Error())
				return
			}
			request.Value = string(encoded)
			err = sender.Send(ctx, &request)
		}
	}
}

func (c *Controller) runServiceSync(syncUpdate chan *ServiceSyncUpdate) error {
	ctx := context.Background()

	log.Println("Establishing connection to skupper transport service")

	client, err := amqp.Dial("amqps://skupper-router:5671", amqp.ConnSASLAnonymous(), amqp.ConnMaxFrameSize(4294967295), amqp.ConnTLSConfig(c.tlsConfig))
	if err != nil {
		return fmt.Errorf("Failed to create amqp connection: %w", err)
	}
	c.amqpClient = client
	defer c.amqpClient.Close()

	c.amqpSession, err = c.amqpClient.NewSession()
	if err != nil {
		return fmt.Errorf("Failed to create amqp session: %w", err)
	}

	receiver, err := c.amqpSession.NewReceiver(
		amqp.LinkSourceAddress(types.ServiceSyncAddress),
		amqp.LinkCredit(10),
	)
	if err != nil {
		return fmt.Errorf("Failed to create amqp receiver: %w", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(ctx, 1*time.Second)
		receiver.Close(ctx)
		cancel()
	}()

	sendLocal := make(chan bool)
	go c.syncSender(sendLocal)

	for {
		var ok bool
		var origin string
		msg, err := receiver.Receive(ctx)
		if err != nil {
			return fmt.Errorf("Failed reading message from service sync %w", err)
		}
		// Decode message as it is either a request to send update
		// or it is a receipt that needs to be reconciled
		msg.Accept()
		subject := msg.Properties.Subject

		if subject == "service-sync-request" {
			log.Println("Controller received service sync request")
			sendLocal <- true
		} else if subject == "service-sync-update" {
			if origin, ok = msg.ApplicationProperties["origin"].(string); ok {
				if origin != c.origin {
					if updates, ok := msg.Value.(string); ok {
						defs := []types.ServiceInterface{}
						err := json.Unmarshal([]byte(updates), &defs)
						if err == nil {
							indexed := make(map[string]types.ServiceInterface)
							log.Printf("Received service-sync-update from %s: %s\n", origin, updates)
							for _, def := range defs {
								def.Origin = origin
								indexed[def.Address] = def
							}
							syncUpdate <- &ServiceSyncUpdate{
								origin:  origin,
								indexed: indexed,
							}
						}
					}
				}
			} else {
				log.Println("Skupper service sync update type assertion error")
			}
		} else {
			log.Println("Service sync subject not valid")
		}
	}
	return nil
}
