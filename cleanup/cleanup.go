package cleanup

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	k8sv1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/printers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"k8s.io/client-go/tools/clientcmd/api/latest"
	"k8s.io/client-go/util/homedir"
	klog "k8s.io/klog/v2"

	// Load client auth plugins
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

var (
	cleanupExample = `
	# prints the cleaned kubeconfig to stdout, similar to running: kubectl config view
	%[1]s config-cleanup

	# cleanup and save the result
	%[1]s config-cleanup --raw > ./kubeconfig-clean.yaml

	# cleanup and print the configs that were removed
	%[1]s config-cleanup --print-removed --raw > ./kubeconfig-removed.yaml

	# print only the context names that were removed
	%[1]s config-cleanup --print-removed -o=jsonpath='{ range.contexts[*] }{ .name }{"\n"}'
`
)

// Options holds configs used to cleanup a kubeconfig file
type Options struct {
	CleanupIgnoreConfig *k8sv1.ConfigMap
	IgnoreContexts      []string
	// TODO
	// IgnoreUsers
	// IgnoreClusters

	ConnectTimeoutSeconds int
	KubeconfigPath        string
	CleanupUsers          bool
	CleanupClusters       bool
	PrintRaw              bool
	PrintRemoved          bool

	raw     *clientcmdapi.Config // the starting kubeconfig
	result  *clientcmdapi.Config // holds configs that are kept
	removed *clientcmdapi.Config // holds configs that were removed

	ioStreams  genericclioptions.IOStreams
	printFlags *genericclioptions.PrintFlags
	printObj   printers.ResourcePrinterFunc
}

// NewCmdCleanup provides a cobra command wrapping CleanupOptions
func NewCmdCleanup(in io.Reader, out, errout io.Writer) *cobra.Command {
	streams := genericclioptions.IOStreams{In: in, Out: out, ErrOut: errout}
	opts := &Options{
		ioStreams:  streams,
		printFlags: genericclioptions.NewPrintFlags("").WithDefaultOutput("yaml"),

		ConnectTimeoutSeconds: int(10),
		CleanupClusters:       false,
		CleanupUsers:          false,
		PrintRaw:              false,
		PrintRemoved:          false,
	}

	rootCmd := &cobra.Command{
		Use:   "config-cleanup",
		Short: "Attempts to connect to each cluster defined in contexts and removes the ones that fail",
		// Long: "" // TODO: Add long description
		Example: fmt.Sprintf(cleanupExample, "kubectl"),
		CompletionOptions: cobra.CompletionOptions{
			HiddenDefaultCmd: true,
		},
		Version: "v0.6.0-dev-FIXME", // TODO: Add real version information
		RunE: func(c *cobra.Command, args []string) error {
			if err := opts.init(c, args); err != nil {
				return err
			}
			if err := opts.Run(); err != nil {
				return err
			}

			return nil
		},
	}

	// We are hiding the help sub-command here because cobra also
	// injects the -h/--help flag automatically.
	// https://github.com/spf13/cobra/issues/587#issuecomment-810159087
	rootCmd.SetHelpCommand(&cobra.Command{Hidden: true})

	// *NOTE* You must call AddCommand at least once to register
	// the completion/help builtin commands (even though we're hiding the help command anyway)
	// cobra bug? https://github.com/spf13/cobra/issues/1915#issuecomment-1434500674
	rootCmd.AddCommand(&cobra.Command{Hidden: true})

	// Add klog flags but mark them as hidden
	klogFlags := flag.NewFlagSet("config-cleanup", flag.ExitOnError)
	klog.InitFlags(klogFlags)
	rootCmd.Flags().AddGoFlagSet(klogFlags)
	rootCmd.Flags().VisitAll(func(f *pflag.Flag) { f.Hidden = true })

	rootCmd.Flags().IntVarP(&opts.ConnectTimeoutSeconds, "timeout", "t", opts.ConnectTimeoutSeconds, "Seconds to wait for a response from the server before continuing cleanup")
	rootCmd.Flags().StringVar(&opts.KubeconfigPath, "kubeconfig", opts.KubeconfigPath, "Specify a kubeconfig file to cleanup")
	rootCmd.Flags().BoolVar(&opts.CleanupClusters, "clusters", opts.CleanupClusters, "Cleanup cluster entries which are not specified by a context")
	rootCmd.Flags().BoolVar(&opts.CleanupUsers, "users", opts.CleanupUsers, "Cleanup user entries which are not specified by a context")
	rootCmd.Flags().BoolVar(&opts.PrintRaw, "raw", opts.PrintRaw, "Print the raw contents of the kubeconfig after cleanup, suitable for piping to a new file")
	rootCmd.Flags().BoolVar(&opts.PrintRemoved, "print-removed", opts.PrintRemoved, "Print the removed contents of the kubeconfig after cleanup")
	return rootCmd
}

// init sets all information required for cleaning up the current KUBECONFIG
func (o *Options) init(cmd *cobra.Command, args []string) error {
	if len(args) != 0 {
		return fmt.Errorf("unexpected arguments: %v", args)
	}

	printer, err := o.printFlags.ToPrinter()
	if err != nil {
		return err
	}
	o.printObj = printer.PrintObj

	// Define kubeconfig precedence from lowest to highest
	// ~/.kube/config -> $KUBECONFIG -> --kubeconfig
	if o.KubeconfigPath == "" {
		if home := homedir.HomeDir(); home != "" {
			o.KubeconfigPath = filepath.Join(home, ".kube", "config")
		}
		if envConfig := os.Getenv("KUBECONFIG"); envConfig != "" {
			o.KubeconfigPath = envConfig
		}
	}

	// Parse config-cleanup.ignore
	if home := homedir.HomeDir(); home != "" {
		data, err := os.ReadFile(filepath.Join(home, ".kube", "config-cleanup.ignore"))

		// Return if the error was anything besides that the file does not exist
		if err != nil && !os.IsNotExist(err) {
			return err
		}

		ignoreconfig, err := loadConfigMap(data)
		if err != nil {
			return err
		}
		o.CleanupIgnoreConfig = ignoreconfig
		contexts, ok := ignoreconfig.Data["contexts"]
		if ok {
			o.IgnoreContexts = strings.Fields(contexts)
		}
	}

	config, err := clientcmd.LoadFromFile(o.KubeconfigPath)
	if err != nil {
		return err
	}
	o.raw = config

	o.result = clientcmdapi.NewConfig()
	o.removed = clientcmdapi.NewConfig()
	o.result.Preferences = o.raw.Preferences
	o.removed.Preferences = o.raw.Preferences
	o.result.Extensions = o.raw.Extensions
	o.removed.Extensions = o.raw.Extensions

	return nil
}

func testConnection(
	contexts <-chan string,
	success chan<- string,
	failure chan<- string,
	completed *int32, o *Options,
) {

	for ctx := range contexts {

		ignore := Contains(o.IgnoreContexts, ctx)
		if ignore {
			success <- ctx
			atomic.AddInt32(completed, 1)
			continue
		}

		clientset, err := o.NewRestClientForContext(ctx)
		if err != nil {
			klog.Infof("%v", err)
			failure <- ctx
			atomic.AddInt32(completed, 1)
			continue
		}

		_, err = clientset.Discovery().ServerVersion()
		if err != nil {
			klog.Infof("%v", err)
			failure <- ctx
		} else {
			success <- ctx
		}
		atomic.AddInt32(completed, 1)
	}
}

// Run cleans up the user's current KUBECONFIG and prints the result to stdout
func (o *Options) Run() error {
	contexts := make(chan string, 100)
	success := make(chan string)
	failure := make(chan string)

	var completed int32

	// TODO: this is arbitrary, add a concurrency limit flag?
	numWorkers := len(o.raw.Contexts)
	if numWorkers > 25 {
		numWorkers = 25
	}
	for w := 0; w <= numWorkers; w++ {
		go testConnection(contexts, success, failure, &completed, o)
	}

	for ctxname := range o.raw.Contexts {
		contexts <- ctxname
	}
	close(contexts)

	// GH-1: Increase default connection timeout and print progress to stderr
	go func() {
		ticker := time.NewTicker(3 * time.Second)
		for {
			<-ticker.C
			if completed == int32(len(o.raw.Contexts)) {
				klog.Infof("Finished testing %d connections...", completed)
				close(success)
				close(failure)
				return
			}
			klog.Infof("Finished testing %d of %d connections...", completed, len(o.raw.Contexts))
		}
	}()

	for range o.raw.Contexts {
		select {
		case s := <-success:
			o.keepContext(s)
		case f := <-failure:
			o.cleanupContext(f)
		}
	}

	zombieClusters := clustersNotSpecifiedByAContext(o.raw)
	if !o.CleanupClusters {
		for name, cluster := range zombieClusters {
			o.result.Clusters[name] = cluster
		}
	} else {
		for name, cluster := range zombieClusters {
			o.removed.Clusters[name] = cluster
		}
	}

	zombieUsers := usersNotSpecifiedByAContext(o.raw)
	if !o.CleanupUsers {
		for name, user := range zombieUsers {
			o.result.AuthInfos[name] = user
		}
	} else {
		for name, user := range zombieUsers {
			o.removed.AuthInfos[name] = user
		}
	}

	result := o.result
	if o.PrintRemoved {
		result = o.removed
	}

	klog.Flush()

	// GH-2: If nothing is left in output then don't print an empty kubeconfig
	if len(result.Clusters) == 0 && len(result.Contexts) == 0 && len(result.AuthInfos) == 0 {
		return nil
	}

	if !o.PrintRaw {
		clientcmdapi.ShortenConfig(result)
	}

	convertedObj, err := latest.Scheme.ConvertToVersion(result, latest.ExternalVersion)
	if err != nil {
		return err
	}

	return o.printObj(convertedObj, o.ioStreams.Out)
}

// NewRestClientForContext initializes an API server REST client from a given context
func (o *Options) NewRestClientForContext(ctxname string) (*kubernetes.Clientset, error) {

	config := clientcmdapi.NewConfig()
	config.CurrentContext = ctxname
	context := o.raw.Contexts[ctxname]

	authInfo, ok := o.raw.AuthInfos[context.AuthInfo]
	if !ok {
		return nil, fmt.Errorf("authInfo not found for context: %s", ctxname)
	}
	cluster, ok := o.raw.Clusters[context.Cluster]
	if !ok {
		return nil, fmt.Errorf("cluster not found for context: %s", ctxname)
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

func (o *Options) keepContext(ctxname string) {
	context := o.raw.Contexts[ctxname]
	o.result.Contexts[ctxname] = context
	auth, ok := o.raw.AuthInfos[context.AuthInfo]
	if ok {
		o.result.AuthInfos[context.AuthInfo] = auth
	}
	cluster, ok := o.raw.Clusters[context.Cluster]
	if ok {
		o.result.Clusters[context.Cluster] = cluster
	}
}

func (o *Options) cleanupContext(ctxname string) {
	context := o.raw.Contexts[ctxname]
	o.removed.Contexts[ctxname] = context
	auth, ok := o.raw.AuthInfos[context.AuthInfo]
	if ok {
		o.removed.AuthInfos[context.AuthInfo] = auth
	}
	cluster, ok := o.raw.Clusters[context.Cluster]
	if ok {
		o.removed.Clusters[context.Cluster] = cluster
	}
}

// kubeConfigGetter is a noop which returns a function meeting the kubeconfigGetter interface
// which we can use to initialize a rest client with the provided authInfo
// ref: https://github.com/kubernetes/contrib/blob/fbb1430dbec659c81b8a0f7492d14f7caeab7505/kubeform/pkg/provider/provider.go#L300
func kubeConfigGetter(config *clientcmdapi.Config) clientcmd.KubeconfigGetter {
	return func() (*clientcmdapi.Config, error) {
		return config, nil
	}
}

// loadConfigMap takes a byte slice and deserializes the contents into ConfigMap object.
func loadConfigMap(data []byte) (*k8sv1.ConfigMap, error) {
	config := &k8sv1.ConfigMap{}
	decoded, _, err := latest.Codec.Decode(data, &schema.GroupVersionKind{Version: latest.Version, Kind: "ConfigMap"}, config)
	if err != nil {
		return nil, err
	}
	return decoded.(*k8sv1.ConfigMap), nil
}

func clustersNotSpecifiedByAContext(rawconfig *clientcmdapi.Config) map[string]*clientcmdapi.Cluster {
	clustersInUse := []string{}
	for _, context := range rawconfig.Contexts {
		clustersInUse = append(clustersInUse, context.Cluster)
	}
	allClusters := []string{}
	for name := range rawconfig.Clusters {
		allClusters = append(allClusters, name)
	}
	zombies := map[string]*clientcmdapi.Cluster{}
	for _, c := range allClusters {
		if !Contains(clustersInUse, c) {
			zombies[c] = rawconfig.Clusters[c]
		}
	}
	return zombies
}

func usersNotSpecifiedByAContext(rawconfig *clientcmdapi.Config) map[string]*clientcmdapi.AuthInfo {
	authInfosInUse := []string{}
	for _, context := range rawconfig.Contexts {
		authInfosInUse = append(authInfosInUse, context.AuthInfo)
	}
	allAuthInfos := []string{}
	for name := range rawconfig.AuthInfos {
		allAuthInfos = append(allAuthInfos, name)
	}
	zombies := map[string]*clientcmdapi.AuthInfo{}
	for _, a := range allAuthInfos {
		if !Contains(authInfosInUse, a) {
			zombies[a] = rawconfig.AuthInfos[a]
		}
	}
	return zombies
}

// Contains util, whether str x is in the slice a
func Contains(a []string, x string) bool {
	for _, n := range a {
		if x == n {
			return true
		}
	}
	return false
}
