package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/genericclioptions/printers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"k8s.io/client-go/tools/clientcmd/api/latest"
	"k8s.io/kubernetes/pkg/kubectl/util/i18n"
	// Uncomment to load all auth plugins
	// _ "k8s.io/client-go/plugin/pkg/client/auth"
)

var (
	cleanupExample = `
	# cleanup the currently active KUBECONFIG, print to stdout
	%[1]s cleanup

	# cleanup clusters and users not specified by a context
	%[1]s cleanup --zombie-clusters --zombie-users
`
)

// CleanupOptions holds configs used to cleanup a kubeconfig file
type CleanupOptions struct {
	ConfigFlags *genericclioptions.ConfigFlags

	PrintFlags  *genericclioptions.PrintFlags
	PrintObject printers.ResourcePrinterFunc

	ResultingConfig *clientcmdapi.Config

	CleanupZombieUsers    bool
	CleanupZombieClusters bool

	// do not cleanup the context if the apiserver returns a 403, defaults to true
	IgnorePermissionDenied bool

	RawConfig clientcmdapi.Config

	genericclioptions.IOStreams
}

// NewCmdCleanup provides a cobra command wrapping CleanupOptions
func NewCmdCleanup(streams genericclioptions.IOStreams) *cobra.Command {
	o := &CleanupOptions{
		ConfigFlags: genericclioptions.NewConfigFlags(),

		PrintFlags:             genericclioptions.NewPrintFlags("").WithTypeSetter(scheme.Scheme).WithDefaultOutput("yaml"),
		IgnorePermissionDenied: true,

		IOStreams: streams,
	}

	cmd := &cobra.Command{
		Use:          "cleanup [flags]",
		Short:        i18n.T("Cleanup the current KUBECONFIG to get rid of inactive clusters and users"),
		Example:      fmt.Sprintf(cleanupExample, "kubectl"),
		SilenceUsage: true,
		RunE: func(c *cobra.Command, args []string) error {
			if err := o.Complete(c, args); err != nil {
				return err
			}
			if err := o.Validate(); err != nil {
				return err
			}
			if err := o.Run(); err != nil {
				return err
			}

			return nil
		},
	}

	// TODO: expose IgnorePermissionDenied as flag
	cmd.Flags().BoolVarP(&o.CleanupZombieUsers, "users", "u", o.CleanupZombieUsers, "if true, cleanup zombie user entries in the current KUBECONFIG")
	cmd.Flags().BoolVarP(&o.CleanupZombieClusters, "clusters", "c", o.CleanupZombieClusters, "if true, cleanup zombie cluster entries in the current KUBECONFIG")
	o.ConfigFlags.AddFlags(cmd.Flags())
	o.PrintFlags.AddFlags(cmd)

	return cmd
}

// Validate ensures that all required arguments and flag values are provided
func (o *CleanupOptions) Validate() error {

	// placeholder, since there's nothing to validate yet
	return nil
}

// Complete sets all information required for cleaning up the current KUBECONFIG
func (o *CleanupOptions) Complete(cmd *cobra.Command, args []string) error {
	if len(args) != 0 {
		return fmt.Errorf("unexpected arguments: %v", args)
	}

	var err error
	o.RawConfig, err = o.ConfigFlags.ToRawKubeConfigLoader().RawConfig()
	if err != nil {
		return err
	}

	printer, err := o.PrintFlags.ToPrinter()
	if err != nil {
		return err
	}
	o.PrintObject = printer.PrintObj

	// dont do anything with extensions or preferences for now
	o.ResultingConfig = clientcmdapi.NewConfig()
	o.ResultingConfig.Preferences = o.RawConfig.Preferences
	o.ResultingConfig.Extensions = o.RawConfig.Extensions

	return nil
}

// Run cleans up the user's current KUBECONFIG and prints the result to stdout
func (o *CleanupOptions) Run() error {

	// Test all contexts, adding valid contexts, users, and clusters back to the ResultingConfig
	for ctxname, context := range o.RawConfig.Contexts {
		fmt.Printf("Testing connection for context: %s\n", ctxname) // TODO: handle debug logging
		clientset, err := o.RestClientFromContextInfo(ctxname, context)
		if err != nil {
			// TODO: Maintain invalid configs in result?
			// TODO: log error
			continue
		}
		if testConnection(clientset, o.IgnorePermissionDenied) == nil {

		}
	}

	// if o.CleanupZombieClusters {
	// TODO
	// }
	// if o.CleanupZombieUsers {
	// TODO
	// }

	convertedObj, err := latest.Scheme.ConvertToVersion(o.ResultingConfig.DeepCopyObject(), latest.ExternalVersion)
	if err != nil {
		return err
	}

	return o.PrintObject(convertedObj, o.Out)
}

// RestClientFromContextInfo initializes an API server REST client from a given context
func (o *CleanupOptions) RestClientFromContextInfo(ctxname string, context *clientcmdapi.Context) (*kubernetes.Clientset, error) {

	config := clientcmdapi.NewConfig()
	config.CurrentContext = ctxname

	authInfo, ok := o.RawConfig.AuthInfos[context.AuthInfo]
	if !ok {
		return nil, fmt.Errorf("AuthInfo not found for context: %s", ctxname)
	}
	cluster, ok := o.RawConfig.Clusters[context.Cluster]
	if !ok {
		return nil, fmt.Errorf("Cluster not found for context: %s", ctxname)
	}

	config.Contexts[ctxname] = context
	config.AuthInfos[context.AuthInfo] = authInfo
	config.Clusters[context.Cluster] = cluster

	configGetter := kubeConfigGetter(config)
	restConfig, err := clientcmd.BuildConfigFromKubeconfigGetter(cluster.Server, configGetter)
	if err != nil {
		return nil, err
	}

	return kubernetes.NewForConfig(restConfig)
}

// testContextConnection attempts to connect to a kubernetes API server using the provided clientset
func testConnection(clientset *kubernetes.Clientset, ignorePermissionDenied bool) error {
	//TODO: list clusterInfo
	return nil
}

// kubeConfigGetter is a noop which returns a function meeting the kubeconfigGetter interface
// that we can use to initialize a rest client with the provided authInfo
// ref: https://github.com/kubernetes/contrib/blob/fbb1430dbec659c81b8a0f7492d14f7caeab7505/kubeform/pkg/provider/provider.go#L300
func kubeConfigGetter(config *clientcmdapi.Config) clientcmd.KubeconfigGetter {
	return func() (*clientcmdapi.Config, error) {
		return config, nil
	}
}
