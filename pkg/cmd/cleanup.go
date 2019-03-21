package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"k8s.io/client-go/util/homedir"

	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/genericclioptions/printers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"k8s.io/client-go/tools/clientcmd/api/latest"
	"k8s.io/kubernetes/pkg/kubectl/util/i18n"
)

var (
	cleanupExample = `
	# prints the cleaned kubeconfig to stdout, similar to running: kubectl config view
	%[1]s cleanup

	# cleanup and save the result back to the config file
	%[1]s cleanup --save

	# cleanup and print the configs that were removed
	%[1]s cleanup --print-removed --raw > ./kubeconfig-removed.yaml

	# print only the context names that would be removed
	%[1]s cleanup --print-removed -o=jsonpath='{ range.contexts[*] }{ .name }{"\n"}'
`
)

// CleanupOptions holds configs used to cleanup a kubeconfig file
type CleanupOptions struct {
	PrintFlags *genericclioptions.PrintFlags

	PrintObject printers.ResourcePrinterFunc

	RawConfig       *clientcmdapi.Config // the starting kubeconfig
	ResultingConfig *clientcmdapi.Config // holds configs we are keeping
	CleanedUpConfig *clientcmdapi.Config // holds configs that were removed

	CleanupZombieUsers    bool
	CleanupZombieClusters bool
	ConnectTimeoutSeconds int
	KubeconfigPath        string
	PrintRaw              bool
	PrintRemoved          bool
	Save                  bool
	// TODO: Promt before each instead of attempting to connect
	// PromptUser            bool

	genericclioptions.IOStreams
}

// NewCmdCleanup provides a cobra command wrapping CleanupOptions
func NewCmdCleanup(streams genericclioptions.IOStreams) *cobra.Command {
	o := &CleanupOptions{
		PrintFlags: genericclioptions.NewPrintFlags("").WithDefaultOutput("yaml"),

		ConnectTimeoutSeconds: int(3),
		CleanupZombieClusters: false,
		CleanupZombieUsers:    false,
		PrintRaw:              false,
		PrintRemoved:          false,
		Save:                  false,

		IOStreams: streams,
	}

	cmd := &cobra.Command{
		Use:          "cleanup [flags]",
		Short:        i18n.T("Attempts to connect to each cluster defined in contexts and removes the ones that fail"),
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

	cmd.Flags().IntVarP(&o.ConnectTimeoutSeconds, "timeout", "t", o.ConnectTimeoutSeconds, "Seconds to wait for a response from the server before continuing cleanup")
	cmd.Flags().StringVar(&o.KubeconfigPath, "kubeconfig", o.KubeconfigPath, "Specify a kubeconfig file to cleanup")
	// cmd.Flags().BoolVarP(&o.PromptUser, "prompt", "p", o.PromptUser, "Prompt the user before removing entries in the current kubeconfig, do not test the connection first")
	cmd.Flags().BoolVarP(&o.Save, "save", "s", o.Save, "Overwrite to the current kubeconfig file")
	cmd.Flags().BoolVarP(&o.PrintRaw, "raw", "r", o.PrintRaw, "Print the raw contents of the kubeconfig after cleanup, suitable for piping to a new file")
	cmd.Flags().BoolVar(&o.PrintRemoved, "print-removed", o.PrintRemoved, "Print the removed contents of the kubeconfig after cleanup, suitable for piping to a new file")

	o.PrintFlags.AddFlags(cmd)

	return cmd
}

// Validate ensures that all required arguments and flag values are provided
func (o *CleanupOptions) Validate() error {
	return nil
}

// Complete sets all information required for cleaning up the current KUBECONFIG
func (o *CleanupOptions) Complete(cmd *cobra.Command, args []string) error {
	if len(args) != 0 {
		return fmt.Errorf("unexpected arguments: %v", args)
	}

	// Define kubeconfig precedence from lowest to highest
	// ~/.kube/config -> $KUBECONFIG -> --kubeconfig
	if home := homedir.HomeDir(); home != "" {
		o.KubeconfigPath = filepath.Join(home, ".kube", "config")
	}
	if envConfig := os.Getenv("KUBECONFIG"); envConfig != "" {
		o.KubeconfigPath = envConfig
	}
	kubeconfig, err := cmd.Flags().GetString("kubeconfig")
	if err != nil {
		return err
	}
	o.KubeconfigPath = kubeconfig

	config, err := clientcmd.LoadFromFile(o.KubeconfigPath)
	if err != nil {
		return err
	}
	o.RawConfig = config

	printer, err := o.PrintFlags.ToPrinter()
	if err != nil {
		return err
	}
	o.PrintObject = printer.PrintObj

	o.CleanedUpConfig = clientcmdapi.NewConfig()
	o.ResultingConfig = clientcmdapi.NewConfig()
	o.CleanedUpConfig.Preferences = o.RawConfig.Preferences
	o.ResultingConfig.Preferences = o.RawConfig.Preferences
	o.CleanedUpConfig.Extensions = o.RawConfig.Extensions
	o.ResultingConfig.Extensions = o.RawConfig.Extensions

	return nil
}

// Run cleans up the user's current KUBECONFIG and prints the result to stdout
func (o *CleanupOptions) Run() error {

	// Test all contexts, adding valid contexts, users, and clusters back to the ResultingConfig
	for ctxname, context := range o.RawConfig.Contexts {
		clientset, err := o.RestClientFromContextInfo(ctxname, context)
		if err != nil {
			continue
		}

		err = testConnection(clientset)
		if err == nil {
			o.ResultingConfig.Contexts[ctxname] = context
			o.ResultingConfig.AuthInfos[context.AuthInfo] = o.RawConfig.AuthInfos[context.AuthInfo]
			o.ResultingConfig.Clusters[context.Cluster] = o.RawConfig.Clusters[context.Cluster]
		} else {
			o.CleanedUpConfig.Contexts[ctxname] = context
			o.CleanedUpConfig.AuthInfos[context.AuthInfo] = o.RawConfig.AuthInfos[context.AuthInfo]
			o.CleanedUpConfig.Clusters[context.Cluster] = o.RawConfig.Clusters[context.Cluster]
		}
	}

	// We never want to save the contents of --print-removed back to the original file, so save
	// and return before doing any of the print stuff
	if o.Save {
		return clientcmd.WriteToFile(*o.ResultingConfig, o.KubeconfigPath)
	}

	if o.PrintRemoved {
		o.ResultingConfig = o.CleanedUpConfig
	}

	if !o.PrintRaw {
		clientcmdapi.ShortenConfig(o.ResultingConfig)
	}

	result := o.ResultingConfig.DeepCopyObject()
	convertedObj, err := latest.Scheme.ConvertToVersion(result, latest.ExternalVersion)
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
	restConfig.Timeout = time.Duration(o.ConnectTimeoutSeconds) * time.Second

	return kubernetes.NewForConfig(restConfig)
}

// testContextConnection attempts to connect to a kubernetes API server and
// get the API server version using the provided clientset
func testConnection(clientset *kubernetes.Clientset) error {
	_, err := clientset.Discovery().ServerVersion()
	return err
}

// kubeConfigGetter is a noop which returns a function meeting the kubeconfigGetter interface
// which we can use to initialize a rest client with the provided authInfo
// ref: https://github.com/kubernetes/contrib/blob/fbb1430dbec659c81b8a0f7492d14f7caeab7505/kubeform/pkg/provider/provider.go#L300
func kubeConfigGetter(config *clientcmdapi.Config) clientcmd.KubeconfigGetter {
	return func() (*clientcmdapi.Config, error) {
		return config, nil
	}
}
