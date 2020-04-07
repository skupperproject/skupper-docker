package main

import (
	"fmt"
	"time"

	"github.com/skupperproject/skupper-docker/api/types"
	"github.com/skupperproject/skupper-docker/client"
	"github.com/spf13/cobra"
)

func requiredArg(name string) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 {
			return fmt.Errorf("%s must be specified", name)
		}
		if len(args) > 1 {
			return fmt.Errorf("illegal argument: %s", args[1])
		}
		return nil
	}
}

//TODO what should this be
const (
	version = "undefined"
)

func main() {
	var dockerEndpoint string

	var vanRouterCreateOpts types.VanRouterCreateOptions
	var cmdInit = &cobra.Command{
		Use:   "init",
		Short: "Initialize skupper docker installation",
		Long:  `init will setup a router and other supporting objects to provide a functional skupper installation that can be connected to other skupper installations on the VAN`,
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			cli, _ := client.NewClient(dockerEndpoint)
			err := cli.VanRouterCreate(vanRouterCreateOpts)
			if err == nil {
				fmt.Println("Skupper is now installed.  Use 'skupper status' to get more information.")
			} else {
				fmt.Println("Unable to install Skupper:", err.Error())
			}
		},
	}
	cmdInit.Flags().StringVarP(&vanRouterCreateOpts.SkupperName, "id", "", "", "Provide a specific identity for the skupper installation")
	cmdInit.Flags().BoolVarP(&vanRouterCreateOpts.IsEdge, "edge", "", false, "Configure as an edge")
	cmdInit.Flags().BoolVarP(&vanRouterCreateOpts.EnableController, "enable-proxy-controller", "", false, "Setup the proxy controller as well as the router")
	cmdInit.Flags().BoolVarP(&vanRouterCreateOpts.EnableServiceSync, "enable-service-sync", "", true, "Configure proxy controller to particiapte in service sync (not relevant if --enable-proxy-controller is false)")
	cmdInit.Flags().BoolVarP(&vanRouterCreateOpts.EnableConsole, "enable-router-console", "", false, "Enable router console")
	cmdInit.Flags().StringVarP(&vanRouterCreateOpts.AuthMode, "router-console-auth", "", "", "Authentication mode for router console. One of: 'internal', 'unsecured'")
	cmdInit.Flags().StringVarP(&vanRouterCreateOpts.User, "router-console-user", "", "", "Router console user. Valid only when --router-console-auth=internal")
	cmdInit.Flags().StringVarP(&vanRouterCreateOpts.Password, "router-console-password", "", "", "Router console user. Valid only when --router-console-auth=internal")

	var cmdDelete = &cobra.Command{
		Use:   "delete",
		Short: "Delete skupper installation",
		Long:  `delete will delete any skupper related objects`,
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			cli, _ := client.NewClient(dockerEndpoint)
			err := cli.VanRouterRemove()
			if err == nil {
				fmt.Println("Skupper resources now removed")
			} else {
				fmt.Println("Unable to uninstall Skupper resources:", err.Error())
			}
		},
	}

	var clientIdentity string
	var cmdConnectionToken = &cobra.Command{
		Use:   "connection-token <output-file>",
		Short: "Create a connection token file with which another skupper installation can connect to this one",
		Args:  requiredArg("output-file"),
		Run: func(cmd *cobra.Command, args []string) {
			cli, _ := client.NewClient(dockerEndpoint)
			err := cli.VanConnectorTokenCreate(clientIdentity, args[0])
			if err != nil {
				fmt.Println("Unable to generate connection-token: ", err.Error())
			}
		},
	}
	cmdConnectionToken.Flags().StringVarP(&clientIdentity, "client-identity", "i", "skupper", "Provide a specific identity as which connecting skupper installation will be authenticated")

	var vanConnectorCreateOpts types.VanConnectorCreateOptions
	var cmdConnect = &cobra.Command{
		Use:   "connect <connection-token-file>",
		Short: "Connect this skupper installation to that which issued the specified connectionToken",
		Args:  requiredArg("connection-token"),
		Run: func(cmd *cobra.Command, args []string) {
			cli, _ := client.NewClient(dockerEndpoint)
			err := cli.VanConnectorCreate(args[0], vanConnectorCreateOpts)
			if err != nil {
				fmt.Println("Unable to connect to skupper installation: ", err.Error())
			}
		},
	}
	cmdConnect.Flags().StringVarP(&vanConnectorCreateOpts.Name, "connection-name", "", "", "Provide a specific name for the connection (used when removing it with disconnect)")
	cmdConnect.Flags().Int32VarP(&vanConnectorCreateOpts.Cost, "cost", "", 1, "Specify a cost for this connection.")

	var cmdDisconnect = &cobra.Command{
		Use:   "disconnect <name>",
		Short: "Remove specified connection",
		Args:  requiredArg("connection name"),
		Run: func(cmd *cobra.Command, args []string) {
			cli, _ := client.NewClient(dockerEndpoint)
			err := cli.VanConnectorRemove(args[0])
			if err != nil {
				fmt.Println("Unable to disconnect from skupper installation: ", err.Error())
			}
		},
	}

	var cmdListConnectors = &cobra.Command{
		Use:   "list-connectors",
		Short: "List configured outgoing VAN connections",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			cli, _ := client.NewClient(dockerEndpoint)
			connectors, err := cli.VanConnectorList()
			if err == nil {
				if len(connectors) == 0 {
					fmt.Println("There are no connectors defined.")
				} else {
					fmt.Println("Connectors:")
					for _, c := range connectors {
						fmt.Printf("    %s:%s (name=%s)", c.Host, c.Port, c.Name)
						fmt.Println()
					}
				}
			} else {
				// TODO: handle is not found
				fmt.Println("Error, unable to retrieve VAN connections: ", err.Error())
			}
		},
	}

	var waitFor int
	var cmdCheckConnection = &cobra.Command{
		Use:   "check-connection all|<connection-name>",
		Short: "Check whether a connection to another Skupper site is active",
		Args:  requiredArg("connection name"),
		Run: func(cmd *cobra.Command, args []string) {
			cli, _ := client.NewClient(dockerEndpoint)
			var connectors []*types.VanConnectorInspectResponse
			connected := 0

			if args[0] == "all" {
				vcis, err := cli.VanConnectorList()
				if err == nil {
					for _, vci := range vcis {
						connectors = append(connectors, &types.VanConnectorInspectResponse{
							Connector: vci,
							Connected: false,
						})
					}
				} else {
					fmt.Println("Unable to retrieve all connector list:", err.Error())
					return
				}
			} else {
				vci, err := cli.VanConnectorInspect(args[0])
				if err == nil {
					connectors = append(connectors, vci)
					if vci.Connected {
						connected++
					}
				} else {
					fmt.Printf("Unable to inspect connector %s: %s", args[0], err.Error())
					return
				}
			}

			for i := 0; connected < len(connectors) && i < waitFor; i++ {
				for _, c := range connectors {
					vci, err := cli.VanConnectorInspect(c.Connector.Name)
					if err == nil && vci.Connected && c.Connected == false {
						c.Connected = true
						connected++
					}
				}
				time.Sleep(time.Second)
			}

			if len(connectors) == 0 {
				fmt.Println("There are no connectors configured or active")
			} else {
				for _, c := range connectors {
					if c.Connected {
						fmt.Printf("Connection for %s is active", c.Connector.Name)
						fmt.Println()
					} else {
						fmt.Printf("Connection for %s not active", c.Connector.Name)
						fmt.Println()
					}
				}
			}
		},
	}
	cmdCheckConnection.Flags().IntVar(&waitFor, "wait", 1, "The number of seconds to wait for connections to become active")

	var cmdStatus = &cobra.Command{
		Use:   "status",
		Short: "Report the status of the current Skupper site",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			cli, _ := client.NewClient(dockerEndpoint)
			vir, err := cli.VanRouterInspect()
			if err == nil {
				var modedesc string = " in interior mode"
				if vir.Status.Mode == types.TransportModeEdge {
					modedesc = " in edge mode"
				}
				fmt.Printf("VanRouter is enabled %s'.", modedesc)
				if vir.Status.ConnectedSites.Total == 0 {
					fmt.Printf(" It is not connected to any other sites.")
				} else if vir.Status.ConnectedSites.Total == 1 {
					fmt.Printf(" It is connected to 1 other site.")
				} else if vir.Status.ConnectedSites.Total == vir.Status.ConnectedSites.Direct {
					fmt.Printf(" It is connected to %d other sites.", vir.Status.ConnectedSites.Total)
				} else {
					fmt.Printf(" It is connected to %d other sites (%d indirectly).", vir.Status.ConnectedSites.Total, vir.Status.ConnectedSites.Indirect)
				}
				if vir.ExposedServices == 0 {
					fmt.Printf(" It has no exposed services.")
				} else if vir.ExposedServices == 1 {
					fmt.Printf(" It has 1 exposed service.")
				} else {
					fmt.Printf(" It has %d exposed services.", vir.ExposedServices)
				}
				fmt.Println()
			} else {
				fmt.Println("Unable to retrieve skupper status: ", err.Error())
			}
		},
	}

	vanServiceInterfaceCreateOpts := types.VanServiceInterfaceCreateOptions{}
	var cmdExpose = &cobra.Command{
		Use:   "expose <name>",
		Short: "Expose a skupper address and optionall a local target to the skupper network",
		Args:  requiredArg("address"),
		Run: func(cmd *cobra.Command, args []string) {
			cli, _ := client.NewClient(dockerEndpoint)
			err := cli.VanServiceInterfaceCreate(args[0], vanServiceInterfaceCreateOpts)
			if err == nil {
				fmt.Printf("VAN Service Interface Target %s exposed\n", args[0])
			} else {
				fmt.Println("Error, unable to create VAN service interface: ", err.Error())
			}
		},
	}
	cmdExpose.Flags().StringVar(&(vanServiceInterfaceCreateOpts.Protocol), "protocol", "tcp", "The protocol to proxy (tcp, http, or http2)")
	cmdExpose.Flags().StringVar(&(vanServiceInterfaceCreateOpts.Address), "address", "", "The Skupper address to expose")
	cmdExpose.Flags().IntVar(&(vanServiceInterfaceCreateOpts.Port), "port", 0, "The port to expose on")
	cmdExpose.Flags().IntVar(&(vanServiceInterfaceCreateOpts.TargetPort), "target-port", 0, "The port to target on pods")
	cmdExpose.Flags().BoolVar(&(vanServiceInterfaceCreateOpts.Headless), "headless", false, "Expose through a headless service (valid only for a statefulset target)")

	var cmdUnexpose = &cobra.Command{
		Use:   "unexpose <name>",
		Short: "Unexpose container previously exposed via skupper address",
		Args:  requiredArg("address"),
		Run: func(cmd *cobra.Command, args []string) {
			cli, _ := client.NewClient(dockerEndpoint)
			err := cli.VanServiceInterfaceRemove(args[0])
			if err == nil {
				fmt.Printf("VAN Service Interface Target %s unexposed\n", args[0])
			} else {
				fmt.Println("Error, unable to remove VAN service interface: ", err.Error())
			}
		},
	}

	var cmdListExposed = &cobra.Command{
		Use:   "list-exposed",
		Short: "List services exposed over the skupper network",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			cli, _ := client.NewClient(dockerEndpoint)
			vsis, err := cli.VanServiceInterfaceList()
			if err == nil {
				if len(vsis) == 0 {
					fmt.Println("No service interfaces defined")
				} else {
					fmt.Println("Services exposed through Skupper:")
					for _, si := range vsis {
						if len(si.Targets) == 0 {
							fmt.Printf("    %s (%s port %d)", si.Address, si.Protocol, si.Port)
							fmt.Println()
						} else {
							fmt.Printf("    %s (%s port %d) with targets", si.Address, si.Protocol, si.Port)
							fmt.Println()
							for _, t := range si.Targets {
								var name string
								if t.Name != "" {
									name = fmt.Sprintf("name=%s", t.Name)
								}
								fmt.Printf("      => %s %s", t.Selector, name)
								fmt.Println()
							}
						}
					}
				}
			} else {
				fmt.Println("Unable to retrieve service interfaces", err.Error())
			}
		},
	}

	var cmdVersion = &cobra.Command{
		Use:   "version",
		Short: "Report version of skupper cli and services",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			cli, _ := client.NewClient(dockerEndpoint)
			vir, err := cli.VanRouterInspect()
			if err == nil {
				fmt.Printf("client version               %s\n", version)
				fmt.Printf("transport version            %s\n", vir.TransportVersion)
				fmt.Printf("controller version           %s\n", vir.ControllerVersion)
			} else {
				fmt.Println("Unable to retrieve skupper component versions: ", err.Error())
			}
		},
	}

	var rootCmd = &cobra.Command{Use: "skupper"}
	rootCmd.Version = version
	rootCmd.AddCommand(cmdInit, cmdDelete, cmdConnectionToken, cmdConnect, cmdDisconnect, cmdCheckConnection, cmdListConnectors, cmdExpose, cmdUnexpose, cmdListExposed, cmdStatus, cmdVersion)
	rootCmd.PersistentFlags().StringVarP(&dockerEndpoint, "endpoint", "e", "", "docker endpoint to use")
	rootCmd.Execute()
}
