package main

import (
	"fmt"
	"os"
	"time"

	"github.com/skupperproject/skupper-docker/api/types"
	"github.com/skupperproject/skupper-docker/client"
	"github.com/spf13/cobra"
)

var version = "undefined"

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

func exposeTarget() func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 {
			return fmt.Errorf("expose type must be specified (e.g. 'skupper-docker expose container' or 'skupper-docker expose host-service')")
		}
		if args[0] == "container" && len(args) < 2 {
			return fmt.Errorf("expose container target must be specified (e.g. 'skupper-docker expose container <name>'")
		}
		if args[0] == "container" && len(args) > 2 {
			return fmt.Errorf("illegal argument for expose container: %s", args[2])
		}
		if args[0] == "host-service" && len(args) > 1 {
			return fmt.Errorf("illegal argument for expose host-service: %s", args[1])
		}
		if args[0] != "container" && args[0] != "host-service" {
			return fmt.Errorf("expose target type must be one of 'container', or 'host-service'")
		}
		return nil
	}
}

var unexposeOpts types.ServiceInterfaceRemoveOptions

func NewCmdUnexpose(newClient cobraFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:    "unexpose [container <name>|host-service]",
		Short:  "Unexpose container or host process previously exposed through a skupper address",
		Args:   exposeTarget(),
		PreRun: newClient,
		RunE: func(cmd *cobra.Command, args []string) error {
			silenceCobra(cmd)

			if args[0] == "container" {
				err := cli.ServiceInterfaceRemove(args[0], args[1], unexposeOpts)
				if err == nil {
					fmt.Printf("%s %s unexposed\n", args[0], args[1])
				} else {
					return fmt.Errorf("Unable to unbind skupper service: %w", err)
				}
			}
			if args[0] == "host-service" {
				err := cli.ServiceInterfaceRemove(args[0], "", unexposeOpts)
				if err == nil {
					fmt.Printf("host-service %s unexposed\n", unexposeOpts.Address)
				} else {
					return fmt.Errorf("Unable to unbind skupper service: %w", err)
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&unexposeOpts.Address, "address", "", "Skupper address the target was exposed as")

	return cmd
}

func NewCmdListExposed(newClient cobraFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:    "list-exposed",
		Short:  "List services exposed over the Skupper network",
		Args:   cobra.NoArgs,
		PreRun: newClient,
		RunE: func(cmd *cobra.Command, args []string) error {
			silenceCobra(cmd)
			vsis, err := cli.ServiceInterfaceList()
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
					fmt.Println()
					fmt.Println("Aliases for services exposed through Skupper:")
					for _, si := range vsis {
						fmt.Printf("    %s %s", si.Alias, si.Address)
						fmt.Println()
					}
					fmt.Println()
				}
			} else {
				return fmt.Errorf("Could not retrieve services: %w", err)
			}
			return nil
		},
	}
	return cmd
}

func NewCmdVersion(newClient cobraFunc) *cobra.Command {
	// TODO: change to inspect
	cmd := &cobra.Command{
		Use:    "version",
		Short:  "Report the version of the Skupper CLI and services",
		Args:   cobra.NoArgs,
		PreRun: newClient,
		RunE: func(cmd *cobra.Command, args []string) error {
			silenceCobra(cmd)
			vir, err := cli.RouterInspect()
			fmt.Printf("%-30s %s\n", "client version", version)
			if err == nil {
				fmt.Printf("%-30s %s\n", "transport version", vir.TransportVersion)
				fmt.Printf("%-30s %s\n", "controller version", vir.ControllerVersion)
			} else {
				return fmt.Errorf("Unable to retrieve skupper component versions: %w", err)
			}
			return nil
		},
	}
	return cmd
}

func silenceCobra(cmd *cobra.Command) {
	cmd.SilenceUsage = true
}

var routerCreateOpts types.RouterCreateOptions

func NewCmdInit(newClient cobraFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialise skupper-docker installation",
		Long: `Setup a router and other supporting objects to provide a functional skupper
installation that can then be connected to other skupper installations`,
		Args:   cobra.NoArgs,
		PreRun: newClient,
		RunE: func(cmd *cobra.Command, args []string) error {
			silenceCobra(cmd)
			err := cli.RouterCreate(routerCreateOpts)
			if err != nil {
				return err
			}
			fmt.Println("Skupper is now installed.  Use 'skupper status' to get more information.")
			return nil
		},
	}
	cmd.Flags().StringVarP(&routerCreateOpts.SkupperName, "id", "", "", "Provide a specific identity for the skupper installation")
	cmd.Flags().BoolVarP(&routerCreateOpts.IsEdge, "edge", "", false, "Configure as an edge")
	cmd.Flags().BoolVarP(&routerCreateOpts.EnableController, "enable-proxy-controller", "", false, "Setup the proxy controller as well as the router")
	cmd.Flags().BoolVarP(&routerCreateOpts.EnableServiceSync, "enable-service-sync", "", true, "Configure proxy controller to particiapte in service sync (not relevant if --enable-proxy-controller is false)")
	cmd.Flags().BoolVarP(&routerCreateOpts.EnableRouterConsole, "enable-router-console", "", false, "Enable router console")
	cmd.Flags().BoolVarP(&routerCreateOpts.EnableConsole, "enable-console", "", false, "Enable skupper console")
	cmd.Flags().StringVarP(&routerCreateOpts.AuthMode, "console-auth", "", "", "Authentication mode for console(s). One of: 'internal', 'unsecured'")
	cmd.Flags().StringVarP(&routerCreateOpts.User, "console-user", "", "", "Router console user. Valid only when --console-auth=internal")
	cmd.Flags().StringVarP(&routerCreateOpts.Password, "console-password", "", "", "Skupper console user. Valid only when --router-console-auth=internal")

	return cmd
}

func NewCmdDelete(newClient cobraFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:    "delete",
		Short:  "Delete skupper installation",
		Long:   `delete will delete any skupper related objects`,
		Args:   cobra.NoArgs,
		PreRun: newClient,
		RunE: func(cmd *cobra.Command, args []string) error {
			silenceCobra(cmd)
			errors := cli.RouterRemove()
			if len(errors) > 0 {
				fmt.Println("Error(s) encountered removing skupper resources:")
				for _, err := range errors {
					fmt.Println("        ", err.Error())
				}
				return errors[0]
			} else {
				fmt.Println("Skupper resources are now removed")
			}
			return nil
		},
	}
	return cmd
}

var clientIdentity string

func NewCmdConnectionToken(newClient cobraFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:    "connection-token <output-file>",
		Short:  "Create a connection token.  The 'connect' command uses the token to establish a connection from a remote Skupper site.",
		Args:   cobra.ExactArgs(1),
		PreRun: newClient,
		RunE: func(cmd *cobra.Command, args []string) error {
			silenceCobra(cmd)
			err := cli.ConnectorTokenCreate(clientIdentity, args[0])
			if err != nil {
				return fmt.Errorf("Failed to create connection token: %w", err)
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&clientIdentity, "client-identity", "i", "skupper", "Provide a specific identity as which connecting skupper installation will be authenticated")

	return cmd
}

var connectorCreateOpts types.ConnectorCreateOptions

func NewCmdConnect(newClient cobraFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:    "connect <connection-token-file>",
		Short:  "Connect this skupper installation to that which issued the specified connectionToken",
		Args:   cobra.ExactArgs(1),
		PreRun: newClient,
		RunE: func(cmd *cobra.Command, args []string) error {
			silenceCobra(cmd)
			// TODO: get site config
			name, err := cli.ConnectorCreate(args[0], connectorCreateOpts)
			if err != nil {
				return fmt.Errorf("Failed to create connection: %w", err)
			}
			fmt.Printf("Skupper connector %s configured\n", name)
			return nil
		},
	}
	cmd.Flags().StringVarP(&connectorCreateOpts.Name, "connection-name", "", "", "Provide a specific name for the connection (used when removing it with disconnect)")
	cmd.Flags().Int32VarP(&connectorCreateOpts.Cost, "cost", "", 1, "Specify a cost for this connection.")

	return cmd
}

func NewCmdDisconnect(newClient cobraFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:    "disconnect <name>",
		Short:  "Remove specified connection",
		Args:   cobra.ExactArgs(1),
		PreRun: newClient,
		RunE: func(cmd *cobra.Command, args []string) error {
			silenceCobra(cmd)
			err := cli.ConnectorRemove(args[0])
			if err != nil {
				return fmt.Errorf("Failed to remove connection: %w", err)
			}
			fmt.Println("Connection '" + args[0] + "' has been removed")

			return nil
		},
	}
	return cmd
}

func NewCmdListConnectors(newClient cobraFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:    "list-connectors",
		Short:  "List configured outgoing connections",
		Args:   cobra.NoArgs,
		PreRun: newClient,
		RunE: func(cmd *cobra.Command, args []string) error {
			silenceCobra(cmd)
			connectors, err := cli.ConnectorList()
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
				return fmt.Errorf("Unable to retrieve connections: %w", err)
			}
			return nil
		},
	}
	return cmd
}

var waitFor int

func NewCmdCheckConnection(newClient cobraFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:    "check-connection all|<connection-name>",
		Short:  "Check whether a connection to another Skupper site is active",
		Args:   cobra.ExactArgs(1),
		PreRun: newClient,
		RunE: func(cmd *cobra.Command, args []string) error {
			silenceCobra(cmd)

			var connectors []*types.ConnectorInspectResponse
			connected := 0

			if args[0] == "all" {
				vcis, err := cli.ConnectorList()
				if err == nil {
					for _, vci := range vcis {
						connectors = append(connectors, &types.ConnectorInspectResponse{
							Connector: vci,
							Connected: false,
						})
					}
				}
			} else {
				vci, err := cli.ConnectorInspect(args[0])
				if err == nil {
					connectors = append(connectors, vci)
					if vci.Connected {
						connected++
					}
				}
			}

			for i := 0; connected < len(connectors) && i < waitFor; i++ {
				for _, c := range connectors {
					vci, err := cli.ConnectorInspect(c.Connector.Name)
					if err == nil && vci.Connected && c.Connected == false {
						c.Connected = true
						connected++
					}
				}
				time.Sleep(time.Second)
			}

			if len(connectors) == 0 {
				if args[0] == "all" {
					fmt.Println("There are no connectors configured or active")
				} else {
					fmt.Printf("The connector %s is not configured or active\n", args[0])
				}
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
			return nil
		},
	}
	cmd.Flags().IntVar(&waitFor, "wait", 1, "The number of seconds to wait for connections to become active")

	return cmd

}

func NewCmdStatus(newClient cobraFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:    "status",
		Short:  "Report the status of the current Skupper site",
		Args:   cobra.NoArgs,
		PreRun: newClient,
		RunE: func(cmd *cobra.Command, args []string) error {
			silenceCobra(cmd)
			vir, err := cli.RouterInspect()
			if err == nil {
				var modedesc string = " in interior mode"
				if vir.Status.Mode == types.TransportModeEdge {
					modedesc = " in edge mode"
				}
				fmt.Printf("Skupper is enabled %s'.", modedesc)
				if vir.Status.State != "running" {
					fmt.Printf(" Status %s...", vir.Status.State)
				} else {
					if len(vir.Status.ConnectedSites.Warnings) > 0 {
						for _, w := range vir.Status.ConnectedSites.Warnings {
							fmt.Printf("Warning: %s", w)
							fmt.Println()
						}
					}
					if vir.Status.ConnectedSites.Total == 0 {
						fmt.Printf(" It is not connected to any other sites.")
					} else if vir.Status.ConnectedSites.Total == 1 {
						fmt.Printf(" It is connected to 1 other site.")
					} else if vir.Status.ConnectedSites.Total == vir.Status.ConnectedSites.Direct {
						fmt.Printf(" It is connected to %d other sites.", vir.Status.ConnectedSites.Total)
					} else {
						fmt.Printf(" It is connected to %d other sites (%d indirectly).", vir.Status.ConnectedSites.Total, vir.Status.ConnectedSites.Indirect)
					}
				}
				if vir.ExposedServices == 0 {
					fmt.Printf(" It has no exposed services.")
				} else if vir.ExposedServices == 1 {
					fmt.Printf(" It has 1 exposed service.")
				} else {
					fmt.Printf(" It has %d exposed services.", vir.ExposedServices)
				}
				//TODO: provide console url
				fmt.Println()
			} else {
				return fmt.Errorf("Unable to retrieve skupper status: %w", err)
			}
			return nil
		},
	}
	return cmd
}

var exposeOpts types.ServiceInterfaceCreateOptions

func NewCmdExpose(newClient cobraFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:    "expose [deployment <name>|pods <selector>|statefulset <statefulsetname>|service <name>]",
		Short:  "Expose a set of pods through a Skupper address",
		Args:   exposeTarget(),
		PreRun: newClient,
		RunE: func(cmd *cobra.Command, args []string) error {
			silenceCobra(cmd)

			if args[0] == "container" {
				err := cli.ServiceInterfaceCreate(args[0], args[1], exposeOpts)
				if err != nil {
					return fmt.Errorf("Unable to expose container: %w", err)
				} else {
					fmt.Printf("%s %s exposed as %s\n", args[0], args[1], exposeOpts.Address)
				}
			}
			if args[0] == "host-service" {
				err := cli.ServiceInterfaceCreate(args[0], "", exposeOpts)
				if err != nil {
					return fmt.Errorf("Unable to expose container: %w", err)
				} else {
					fmt.Printf("%s exposed as %s\n", args[0], exposeOpts.Address)
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&(exposeOpts.Protocol), "protocol", "tcp", "The protocol to proxy (tcp, http, or http2)")
	cmd.Flags().StringVar(&(exposeOpts.Address), "address", "", "The Skupper address to expose")
	cmd.Flags().IntVar(&(exposeOpts.Port), "port", 0, "The port to expose on")
	cmd.Flags().IntVar(&(exposeOpts.TargetPort), "target-port", 0, "The port to target on container or process")

	return cmd
}

type cobraFunc func(cmd *cobra.Command, args []string)

func newClient(cmd *cobra.Command, args []string) {
	cli, _ = client.NewClient(dockerEndpoint)
}

var rootCmd *cobra.Command
var dockerEndpoint string
var cli types.VanClientInterface

func init() {

	cmdInit := NewCmdInit(newClient)
	cmdDelete := NewCmdDelete(newClient)
	cmdConnectionToken := NewCmdConnectionToken(newClient)
	cmdConnect := NewCmdConnect(newClient)
	cmdDisconnect := NewCmdDisconnect(newClient)
	cmdListConnectors := NewCmdListConnectors(newClient)
	cmdCheckConnection := NewCmdCheckConnection(newClient)
	cmdStatus := NewCmdStatus(newClient)
	cmdExpose := NewCmdExpose(newClient)
	cmdUnexpose := NewCmdUnexpose(newClient)
	cmdListExposed := NewCmdListExposed(newClient)
	cmdVersion := NewCmdVersion(newClient)

	rootCmd = &cobra.Command{Use: "skupper-docker"}
	rootCmd.Version = version
	rootCmd.AddCommand(cmdInit,
		cmdDelete,
		cmdConnectionToken,
		cmdConnect,
		cmdDisconnect,
		cmdListConnectors,
		cmdCheckConnection,
		cmdStatus,
		cmdExpose,
		cmdUnexpose,
		cmdListExposed,
		cmdVersion)
	rootCmd.PersistentFlags().StringVarP(&dockerEndpoint, "endpoint", "e", "", "docker endpoint to use")
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
