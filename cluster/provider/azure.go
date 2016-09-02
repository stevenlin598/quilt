package provider

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"os/user"
	"strings"
	"sync"
	"time"

	"github.com/NetSys/quilt/constants"
	"github.com/NetSys/quilt/db"
	"github.com/NetSys/quilt/join"
	"github.com/NetSys/quilt/stitch"

	"github.com/Azure/azure-sdk-for-go/arm/compute"
	"github.com/Azure/azure-sdk-for-go/arm/network"
	"github.com/Azure/azure-sdk-for-go/arm/resources/resources"
	"github.com/Azure/azure-sdk-for-go/arm/storage"
	"github.com/Azure/go-autorest/autorest/azure"

	uuid "github.com/satori/go.uuid"
)

const (
	credentialsPath         = "/.azure/credentials.json"
	imagePublisher          = "Canonical"
	imageOffer              = "UbuntuServer"
	imageSku                = "15.10"
	imageVersion            = "15.10.201511161"
	resourceGroupName       = "quilt"
	resourceGroupLocation   = "centralus"
	subnetName              = "quiltsubnet"
	nsTag                   = "quilt-namespace"
	vmTag                   = "quilt-vm"
	vnetAddressPrefix       = "10.0.0.0/8"
	vnetSubnetAddressPrefix = "10.0.0.0/8"
)

type azureCluster struct {
	azureClient    azureAPI
	namespace      string
	subscriptionID string
}

type azureCredentials struct {
	clientID       string
	clientSecret   string
	tenantID       string
	subscriptionID string
}

// A securityRuleSlice allows for slices of Collections to be used in joins.
type securityRuleSlice []network.SecurityRule

// Create an Azure cluster.
func (clst *azureCluster) Connect(namespace string) error {
	if namespace == "" {
		return errors.New("namespace cannot be empty")
	}
	clst.namespace = namespace

	cred, err := clst.loadCredentials()
	if err != nil {
		return err
	}
	clst.subscriptionID = cred.subscriptionID

	spt, err := clst.generateOAuthToken(cred)
	if err != nil {
		return err
	}
	clst.azureClient = newAzureClient(clst.subscriptionID, spt)

	return clst.configureResourceGroup()
}

// Retrieve list of instances.
func (clst *azureCluster) List() ([]Machine, error) {
	var mList []Machine

	result, err := clst.azureClient.vmList(resourceGroupName)
	if err != nil {
		return nil, err
	}

	for _, vm := range *result.Value {
		if vm.Tags == nil {
			continue
		}

		if !clst.validateResourceTag(*vm.Tags, nsTag, clst.namespace) {
			continue
		}

		nicName := *vm.Name + "-nic"

		iface, err := clst.azureClient.ifaceGet(resourceGroupName, nicName, "")
		if err != nil {
			return nil, err
		}
		ifaceIPConfig := *iface.Properties.IPConfigurations
		if len(ifaceIPConfig) == 0 {
			return nil, errors.New("could not retrieve private IP address")
		}
		privateIP := *ifaceIPConfig[0].Properties.PrivateIPAddress

		pip, err := clst.azureClient.publicIPGet(resourceGroupName, nicName, "")
		if err != nil {
			return nil, err
		}
		publicIP := *pip.Properties.IPAddress

		vmSize := string(vm.Properties.HardwareProfile.VMSize)

		mList = append(mList, Machine{
			ID:        *vm.Name,
			PublicIP:  publicIP,
			PrivateIP: privateIP,
			Provider:  db.Azure,
			Region:    *vm.Location,
			Size:      vmSize,
		})
	}

	return mList, nil
}

// Boot Azure instances (blocking by calling instanceNew).
func (clst *azureCluster) Boot(bootSet []Machine) error {
	storageAccounts, err := clst.listStorageAccounts()
	if err != nil {
		return err
	}

	securityGroups, err := clst.listSecurityGroups()
	if err != nil {
		return err
	}

	virtualNetworks, err := clst.listVirtualNetworks()
	if err != nil {
		return err
	}

	// Map locations to subnets.
	subnets := make(map[string]network.Subnet)

	// For each region, we need:
	// 1. A globally unique storage account.
	// 2. A subscription-wide unique virtual network.
	// 3. A subscription-wide unique security group.
	regions := make(map[string]struct{})
	for _, m := range bootSet {
		if _, ok := regions[m.Region]; ok {
			continue
		}
		regions[m.Region] = struct{}{}

		if _, ok := storageAccounts[m.Region]; !ok {
			storageAccount, err := clst.configureStorageAccount(m.Region)
			if err != nil {
				return err
			}
			storageAccounts[m.Region] = storageAccount
		}

		vnetName := fmt.Sprintf("quiltvnet-%s-%s", clst.namespace, m.Region)
		subnets[m.Region] = clst.configureSubnet(vnetName)

		if _, ok := virtualNetworks[m.Region]; !ok {
			virtualNetwork, err := clst.configureVirtualNetwork(vnetName,
				m.Region, subnets[m.Region])
			if err != nil {
				return err
			}
			virtualNetworks[m.Region] = virtualNetwork
		}

		if _, ok := securityGroups[m.Region]; !ok {
			securityGroupName := fmt.Sprintf("quiltnsg-%s-%s",
				clst.namespace, m.Region)
			securityGroup, err := clst.configureSecurityGroup(
				securityGroupName, m.Region)
			if err != nil {
				return err
			}
			securityGroups[m.Region] = securityGroup
		}
	}

	bootFunc := func(m Machine) error {
		vmName := "quilt-" + uuid.NewV4().String()
		osDiskName := vmName + "-osdisk"
		nicName := vmName + "-nic"

		publicIP, err := clst.configurePublicIP(nicName, m.Region, vmName)
		if err != nil {
			return err
		}

		iface, err := clst.configureNetworkInterface(nicName, m.Region,
			subnets[m.Region], publicIP, securityGroups[m.Region], vmName)
		if err != nil {
			return err
		}

		cloudConfig := cloudConfigUbuntu(m.SSHKeys, "wily")
		if err := clst.configureVirtualMachine(vmName, osDiskName, nicName,
			cloudConfig, m.Size, m.Region, iface); err != nil {
			return err
		}

		return nil
	}

	return forEachMachine(bootFunc, bootSet)
}

func (clst *azureCluster) Stop(machines []Machine) error {
	stopFunc := func(m Machine) error {
		cancel := make(chan struct{})
		if _, err := clst.azureClient.vmDelete(resourceGroupName, m.ID,
			cancel); err != nil {
			return err
		}

		nicName := m.ID + "-nic"
		if _, err := clst.azureClient.ifaceDelete(resourceGroupName, nicName,
			cancel); err != nil {
			return err
		}

		if _, err := clst.azureClient.publicIPDelete(resourceGroupName, nicName,
			cancel); err != nil {
			return err
		}
		return nil
	}

	return forEachMachine(stopFunc, machines)
}

func (clst *azureCluster) ChooseSize(ram stitch.Range, cpu stitch.Range,
	maxPrice float64) string {
	return pickBestSize(constants.AzureDescriptions, ram, cpu, maxPrice)
}

func (clst *azureCluster) loadCredentials() (*azureCredentials, error) {
	u, err := user.Current()
	if err != nil {
		return nil, errors.New("unable to determine current user")
	}

	dir := u.HomeDir + credentialsPath
	f, err := os.Open(dir)
	if err != nil {
		return nil, errors.New("unable to open Azure credentials at " + dir)
	}
	defer f.Close()

	b, err := ioutil.ReadAll(f)
	if err != nil {
		return nil, errors.New("unable to read " + dir)
	}

	credMap := map[string]string{}
	if err = json.Unmarshal(b, &credMap); err != nil {
		return nil, errors.New(dir + " contains invalid JSON")
	}

	var ok bool
	cred := azureCredentials{}

	if cred.clientID, ok = credMap["clientID"]; !ok {
		return nil, errors.New(dir + " contains invalid clientID")
	}

	if cred.clientSecret, ok = credMap["clientSecret"]; !ok {
		return nil, errors.New(dir + " contains invalid clientSecret")
	}

	if cred.tenantID, ok = credMap["tenantID"]; !ok {
		return nil, errors.New(dir + " contains invalid tenantID")
	}

	if cred.subscriptionID, ok = credMap["subscriptionID"]; !ok {
		return nil, errors.New(dir + " contains invalid subscriptionID")
	}

	return &cred, nil
}

func (clst *azureCluster) generateOAuthToken(cred *azureCredentials) (
	*azure.ServicePrincipalToken, error) {
	oauthConfig, err := azure.PublicCloud.OAuthConfigForTenant(cred.tenantID)
	if err != nil {
		return nil, errors.New("failed to configure OAuthConfig for tenant")
	}

	return azure.NewServicePrincipalToken(*oauthConfig, cred.clientID,
		cred.clientSecret, azure.PublicCloud.ResourceManagerEndpoint)
}

func (clst *azureCluster) configureResourceGroup() error {
	resourceGroup := resources.ResourceGroup{
		Name:     stringPtr(resourceGroupName),
		Location: stringPtr(resourceGroupLocation),
	}
	_, err := clst.azureClient.rgCreate(resourceGroupName, resourceGroup)
	return err
}

func (clst *azureCluster) listStorageAccounts() (map[string]storage.Account, error) {
	result, err := clst.azureClient.storageListByRg(resourceGroupName)
	if err != nil {
		return nil, err
	}
	accounts := map[string]storage.Account{}
	for _, account := range *result.Value {
		if !clst.validateResourceTag(*account.Tags, nsTag, clst.namespace) {
			continue
		}
		accounts[*account.Location] = account
	}

	return accounts, nil
}

func randomString(length int) string {
	const chars = "abcdefghijklmnopqrstuvwxyz0123456789"
	result := make([]byte, length)
	rand.Seed(time.Now().UTC().UnixNano())
	for i := 0; i < length; i++ {
		result[i] = chars[rand.Intn(len(chars))]
	}
	return string(result)
}

func (clst *azureCluster) configureStorageAccount(location string) (storage.Account,
	error) {
	var storageAccount storage.Account

	// Storage name needs to be globally unique, with a limit of 24 characters.
	storageName := randomString(24)
	cna, err := clst.azureClient.storageCheckName(
		storage.AccountCheckNameAvailabilityParameters{
			Name: &storageName,
			Type: stringPtr("Microsoft.Storage/storageAccounts")})
	if err != nil {
		return storageAccount, err
	}
	if !*cna.NameAvailable {
		return storageAccount, errors.New("storage account is not available")
	}

	sku := storage.Sku{
		Name: storage.StandardGRS,
		Tier: storage.Standard,
	}

	param := storage.AccountCreateParameters{
		Location: &location,
		Sku:      &sku,
		Tags:     &map[string]*string{nsTag: &clst.namespace},
	}

	cancel := make(chan struct{})
	if _, err := clst.azureClient.storageCreate(resourceGroupName, storageName,
		param, cancel); err != nil {
		return storageAccount, err
	}

	return clst.azureClient.storageGet(resourceGroupName, storageName)
}

func (clst *azureCluster) configureSubnet(vnetName string) network.Subnet {
	id := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/"+
		"Microsoft.Network/virtualNetworks/%s/subnets/%s", clst.subscriptionID,
		resourceGroupName, vnetName, subnetName)

	properties := network.SubnetPropertiesFormat{
		AddressPrefix: stringPtr(vnetSubnetAddressPrefix),
	}

	subnet := network.Subnet{
		ID:         &id,
		Name:       stringPtr(subnetName),
		Properties: &properties,
	}

	return subnet
}

func (clst *azureCluster) listVirtualNetworks() (map[string]network.VirtualNetwork,
	error) {
	result, err := clst.azureClient.vnetList(resourceGroupName)
	if err != nil {
		return nil, err
	}

	vnets := map[string]network.VirtualNetwork{}
	for _, vnet := range *result.Value {
		if !clst.validateResourceTag(*vnet.Tags, nsTag, clst.namespace) {
			continue
		}
		vnets[*vnet.Location] = vnet
	}

	return vnets, nil
}

func (clst *azureCluster) configureVirtualNetwork(vnetName string, location string,
	subnet network.Subnet) (network.VirtualNetwork, error) {
	addressSpace := network.AddressSpace{
		AddressPrefixes: &[]string{vnetAddressPrefix},
	}

	properties := network.VirtualNetworkPropertiesFormat{
		AddressSpace: &addressSpace,
		Subnets:      &[]network.Subnet{subnet},
	}

	virtualNetwork := network.VirtualNetwork{
		Name:       &vnetName,
		Location:   &location,
		Properties: &properties,
		Tags:       &map[string]*string{nsTag: &clst.namespace},
	}

	cancel := make(chan struct{})
	_, err := clst.azureClient.vnetCreate(resourceGroupName, vnetName,
		virtualNetwork, cancel)
	return virtualNetwork, err
}

func (clst *azureCluster) listSecurityGroups() (map[string]network.SecurityGroup,
	error) {
	result, err := clst.azureClient.securityGroupList(resourceGroupName)
	if err != nil {
		return nil, err
	}

	secGroups := map[string]network.SecurityGroup{}
	for _, secGroup := range *result.Value {
		if !clst.validateResourceTag(*secGroup.Tags, nsTag, clst.namespace) {
			continue
		}
		secGroups[*secGroup.Location] = secGroup
	}

	return secGroups, nil
}

func (clst *azureCluster) configureSecurityGroup(securityGroupName string,
	location string) (network.SecurityGroup, error) {
	id := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/"+
		"Microsoft.Network/networkSecurityGroups/%s", clst.subscriptionID,
		resourceGroupName, securityGroupName)

	securityGroup := network.SecurityGroup{
		ID:       &id,
		Name:     &securityGroupName,
		Location: &location,
		Tags:     &map[string]*string{nsTag: &clst.namespace},
	}

	cancel := make(chan struct{})
	_, err := clst.azureClient.securityGroupCreate(resourceGroupName,
		securityGroupName, securityGroup, cancel)
	return securityGroup, err
}

func (clst *azureCluster) configurePublicIP(nicName string, location string,
	vmName string) (network.PublicIPAddress, error) {
	properties := network.PublicIPAddressPropertiesFormat{
		PublicIPAllocationMethod: network.Dynamic,
	}

	id := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/"+
		"Microsoft.Network/publicIPAddresses/%s", clst.subscriptionID,
		resourceGroupName, nicName)

	publicIP := network.PublicIPAddress{
		Name:       &nicName,
		Location:   &location,
		Properties: &properties,
		ID:         &id,
		Tags: &map[string]*string{
			nsTag: &clst.namespace,
			vmTag: &vmName,
		},
	}

	cancel := make(chan struct{})
	_, err := clst.azureClient.publicIPCreate(resourceGroupName, nicName, publicIP,
		cancel)
	return publicIP, err
}

func (clst *azureCluster) configureNetworkInterface(nicName string, location string,
	subnet network.Subnet, pipModel network.PublicIPAddress,
	securityGroup network.SecurityGroup, vmName string) (network.Interface, error) {

	ipConfigProperties := network.InterfaceIPConfigurationPropertiesFormat{
		Subnet:          &subnet,
		PublicIPAddress: &pipModel,
	}

	ipConfig := network.InterfaceIPConfiguration{
		Name:       &nicName,
		Properties: &ipConfigProperties,
	}

	properties := network.InterfacePropertiesFormat{
		IPConfigurations:     &[]network.InterfaceIPConfiguration{ipConfig},
		NetworkSecurityGroup: &securityGroup,
	}

	id := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/"+
		"Microsoft.Network/networkInterfaces/%s", clst.subscriptionID,
		resourceGroupName, nicName)

	iface := network.Interface{
		ID:         &id,
		Name:       &nicName,
		Location:   &location,
		Properties: &properties,
		Tags: &map[string]*string{
			nsTag: &clst.namespace,
			vmTag: &vmName,
		},
	}

	cancel := make(chan struct{})
	_, err := clst.azureClient.ifaceCreate(resourceGroupName, nicName, iface, cancel)
	return iface, err
}

func (clst *azureCluster) configureVirtualMachine(vmName string, osDiskName string,
	nicName string, cloudConfig string, vmSize string, location string,
	iface network.Interface) error {
	hardwareProfile := compute.HardwareProfile{
		VMSize: compute.VirtualMachineSizeTypes(vmSize),
	}

	ifaceRef := compute.NetworkInterfaceReference{
		ID: iface.ID,
	}

	networkProfile := compute.NetworkProfile{
		NetworkInterfaces: &[]compute.NetworkInterfaceReference{ifaceRef},
	}

	storageAccounts, err := clst.listStorageAccounts()
	if err != nil {
		return err
	}

	storageAccount, ok := storageAccounts[location]
	if !ok {
		return errors.New("a storage account is needed for location " + location)
	}

	vhdURI := fmt.Sprintf("http://%s.blob.core.windows.net/vhds/%s.vhd",
		*storageAccount.Name, osDiskName)

	vhd := compute.VirtualHardDisk{
		URI: &vhdURI,
	}

	osDisk := compute.OSDisk{
		Name:         &osDiskName,
		Caching:      compute.ReadWrite,
		CreateOption: compute.FromImage,
		Vhd:          &vhd,
	}

	imageRef := compute.ImageReference{
		Publisher: stringPtr(imagePublisher),
		Offer:     stringPtr(imageOffer),
		Sku:       stringPtr(imageSku),
		Version:   stringPtr(imageVersion),
	}

	storageProfile := compute.StorageProfile{
		ImageReference: &imageRef,
		OsDisk:         &osDisk,
	}

	// We have to set username and password even though we do not need it.
	adminUsername := uuid.NewV4().String()
	adminPassword := uuid.NewV4().String()

	customData := base64.StdEncoding.EncodeToString([]byte(cloudConfig))

	osProfile := compute.OSProfile{
		ComputerName:  &vmName,
		AdminUsername: &adminUsername,
		AdminPassword: &adminPassword,
		CustomData:    &customData,
	}

	properties := compute.VirtualMachineProperties{
		HardwareProfile: &hardwareProfile,
		NetworkProfile:  &networkProfile,
		StorageProfile:  &storageProfile,
		OsProfile:       &osProfile,
	}

	virtualMachine := compute.VirtualMachine{
		Name:       &vmName,
		Location:   &location,
		Properties: &properties,
		Tags:       &map[string]*string{nsTag: &clst.namespace},
	}

	cancel := make(chan struct{})
	_, err = clst.azureClient.vmCreate(resourceGroupName, vmName, virtualMachine,
		cancel)
	return err
}

func (clst *azureCluster) SetACLs(acls []string) error {
	asterisk := "*"
	localInRules := []network.SecurityRule{}
	localOutRules := []network.SecurityRule{}
	for _, acl := range acls {
		address := acl
		// Azure does not allow `/` as security rule name. So we use `-` instead.
		inboundName := strings.Replace(address, "/", "-", -1) + "-in"
		inboundProperties := network.SecurityRulePropertiesFormat{
			Protocol:                 network.Asterisk,
			SourcePortRange:          &asterisk,
			SourceAddressPrefix:      &address,
			DestinationPortRange:     &asterisk,
			DestinationAddressPrefix: &asterisk,
			Access:    network.Allow,
			Direction: network.Inbound,
		}
		inboundRule := network.SecurityRule{
			Name:       &inboundName,
			Properties: &inboundProperties,
		}
		localInRules = append(localInRules, inboundRule)

		outboundName := strings.Replace(address, "/", "-", -1) + "-out"
		outboundProperties := network.SecurityRulePropertiesFormat{
			Protocol:                 network.Asterisk,
			SourcePortRange:          &asterisk,
			SourceAddressPrefix:      &asterisk,
			DestinationPortRange:     &asterisk,
			DestinationAddressPrefix: &address,
			Access:    network.Allow,
			Direction: network.Outbound,
		}
		outboundRule := network.SecurityRule{
			Name:       &outboundName,
			Properties: &outboundProperties,
		}
		localOutRules = append(localOutRules, outboundRule)
	}

	securityGroups, err := clst.listSecurityGroups()
	if err != nil {
		return err
	}

	syncRuleFunc := func(securityGroup network.SecurityGroup) error {
		return clst.syncSecurityGroup(securityGroup, localInRules, localOutRules)
	}

	return forEachSecurityGroup(syncRuleFunc, securityGroups)
}

func (clst *azureCluster) syncSecurityGroup(securityGroup network.SecurityGroup,
	localInRules securityRuleSlice, localOutRules securityRuleSlice) error {
	cloudInRules := []network.SecurityRule{}
	cloudOutRules := []network.SecurityRule{}

	securityGroupName := *securityGroup.Name
	result, err := clst.azureClient.securityRuleList(resourceGroupName,
		securityGroupName)
	if err != nil {
		return err
	}

	for _, rule := range *result.Value {
		if rule.Properties.Direction == network.Inbound {
			cloudInRules = append(cloudInRules, rule)
		} else if rule.Properties.Direction == network.Outbound {
			cloudOutRules = append(cloudOutRules, rule)
		}
	}

	if err := clst.syncSecurityRules(securityGroupName, localInRules,
		cloudInRules); err != nil {
		return err
	}

	return clst.syncSecurityRules(securityGroupName, localOutRules, cloudOutRules)
}

func (clst *azureCluster) syncSecurityRules(securityGroupName string,
	localRules securityRuleSlice, cloudRules securityRuleSlice) error {
	key := func(val interface{}) interface{} {
		property := val.(network.SecurityRule).Properties
		return struct {
			sourcePortRange          string
			sourceAddressPrefix      string
			destinationPortRange     string
			destinationAddressPrefix string
			direction                network.SecurityRuleDirection
		}{
			sourcePortRange:          *property.SourcePortRange,
			sourceAddressPrefix:      *property.SourceAddressPrefix,
			destinationPortRange:     *property.DestinationPortRange,
			destinationAddressPrefix: *property.DestinationAddressPrefix,
			direction:                property.Direction,
		}
	}

	_, addList, deleteList := join.HashJoin(localRules, cloudRules, key, key)

	// Each security rule is required to be assigned one unique priority number
	// Between 100 and 4096.
	newPriorities := []int32{}

	currPriorities := make(map[int32]struct{})
	for _, rule := range cloudRules {
		currPriorities[*rule.Properties.Priority] = struct{}{}
	}

	cancel := make(chan struct{})
	for _, r := range deleteList {
		rule := r.(network.SecurityRule)
		delete(currPriorities, *rule.Properties.Priority)
		if _, err := clst.azureClient.securityRuleDelete(resourceGroupName,
			securityGroupName, *rule.Name, cancel); err != nil {
			return err
		}
	}

	priority := int32(100)
	for range addList {
		foundSlot := false
		for !foundSlot {
			if priority > 4096 {
				return errors.New("max number of security rules reached")
			}
			if _, ok := currPriorities[priority]; !ok {
				newPriorities = append(newPriorities, priority)
				foundSlot = true
			}
			priority++
		}
	}

	for i, r := range addList {
		rule := r.(network.SecurityRule)
		rule.Properties.Priority = &newPriorities[i]
		if _, err := clst.azureClient.securityRuleCreate(resourceGroupName,
			securityGroupName, *rule.Name, rule, cancel); err != nil {
			return err
		}
	}
	return nil
}

func (clst *azureCluster) validateResourceTag(tags map[string]*string, tagKey string,
	expected string) bool {
	if tag := tags[tagKey]; tag == nil || *tag != expected {
		return false
	}
	return true
}

// forEach passes each element in objs through function f concurrently.
func forEach(f func(obj interface{}) error, objs []interface{}) error {
	var wg sync.WaitGroup
	errs := make(chan error, len(objs))
	for _, obj := range objs {
		wg.Add(1)
		go func(obj interface{}) {
			defer wg.Done()
			if err := f(obj); err != nil {
				errs <- err
			}
		}(obj)
	}

	wg.Wait()
	select {
	case err := <-errs:
		return err
	default:
		return nil
	}
}

func forEachMachine(f func(machine Machine) error, machines []Machine) error {
	ms := make([]interface{}, len(machines))
	for i, v := range machines {
		ms[i] = v
	}

	machineFunc := func(obj interface{}) error {
		m := obj.(Machine)
		return f(m)
	}

	return forEach(machineFunc, ms)
}

func forEachSecurityGroup(f func(securityGroup network.SecurityGroup) error,
	securityGroups map[string]network.SecurityGroup) error {
	sgs := make([]interface{}, 0, len(securityGroups))
	for _, v := range securityGroups {
		sgs = append(sgs, v)
	}

	sgFunc := func(obj interface{}) error {
		m := obj.(network.SecurityGroup)
		return f(m)
	}

	return forEach(sgFunc, sgs)
}

// Get method on securityRuleSlice is required for HashJoin.
func (slice securityRuleSlice) Get(i int) interface{} {
	return slice[i]
}

// Len method on securityRuleSlice is required for HashJoin.
func (slice securityRuleSlice) Len() int {
	return len(slice)
}

// stringPtr returns a pointer to the passed string.
func stringPtr(s string) *string {
	return &s
}
