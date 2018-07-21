// Copyright (C) 2017 Battelle Memorial Institute
// All rights reserved.
//
// This software may be modified and distributed under the terms
// of the BSD-2 license.  See the LICENSE file for details.

package ovirt

import (
	"fmt"
	"time"

	"github.com/hashicorp/terraform/helper/resource"
	"github.com/hashicorp/terraform/helper/schema"
	ovirtsdk4 "gopkg.in/imjoey/go-ovirt.v4"
)

func resourceOvirtVM() *schema.Resource {
	return &schema.Resource{
		Create: resourceOvirtVMCreate,
		Read:   resourceOvirtVMRead,
		Update: resourceOvirtVMUpdate,
		Delete: resourceOvirtVMDelete,
		Importer: &schema.ResourceImporter{
			State: schema.ImportStatePassthrough,
		},
		Schema: map[string]*schema.Schema{
			"name": {
				Type:     schema.TypeString,
				Required: true,
			},
			"cluster_id": {
				Type:     schema.TypeString,
				Required: true,
			},
			"template": {
				Type:     schema.TypeString,
				Optional: true,
				Default:  "Blank",
			},
			"memory": {
				Type:     schema.TypeInt,
				Optional: true,
			},
			"cores": {
				Type:     schema.TypeInt,
				Optional: true,
				Default:  1,
			},
			"sockets": {
				Type:     schema.TypeInt,
				Optional: true,
				Default:  1,
			},
			"threads": {
				Type:     schema.TypeInt,
				Optional: true,
				Default:  1,
			},
			"authorized_ssh_key": {
				Type:     schema.TypeString,
				Optional: true,
				Default:  "",
			},
			"attached_disk": {
				Type:     schema.TypeSet,
				Required: true,
				MinItems: 1,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"disk_id": &schema.Schema{
							Type:     schema.TypeString,
							Required: true,
						},
						"active": &schema.Schema{
							Type:     schema.TypeBool,
							Optional: true,
							Default:  true,
						},
						"bootable": &schema.Schema{
							Type:     schema.TypeBool,
							Optional: true,
							Default:  false,
						},
						"interface": &schema.Schema{
							Type:     schema.TypeString,
							Required: true,
						},
						"logical_name": &schema.Schema{
							Type:     schema.TypeString,
							Optional: true,
						},
						"pass_discard": &schema.Schema{
							Type:     schema.TypeBool,
							Optional: true,
						},
						"read_only": &schema.Schema{
							Type:     schema.TypeBool,
							Optional: true,
							Default:  false,
						},
						"use_scsi_reservation": &schema.Schema{
							Type:     schema.TypeBool,
							Optional: true,
							Default:  false,
						},
					},
				},
			},
			"network_interface": {
				Type:     schema.TypeSet,
				Required: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"label": &schema.Schema{
							Type:     schema.TypeString,
							Required: true,
						},
						"network": &schema.Schema{
							Type:     schema.TypeString,
							Optional: true,
						},
						"boot_proto": &schema.Schema{
							Type:     schema.TypeString,
							Required: true,
						},
						"ip_address": &schema.Schema{
							Type:     schema.TypeString,
							Optional: true,
						},
						"subnet_mask": &schema.Schema{
							Type:     schema.TypeString,
							Optional: true,
						},
						"gateway": &schema.Schema{
							Type:     schema.TypeString,
							Optional: true,
						},
						"on_boot": &schema.Schema{
							Type:     schema.TypeBool,
							Optional: true,
							Default:  true,
						},
					},
				},
			},
		},
	}
}

func resourceOvirtVMCreate(d *schema.ResourceData, meta interface{}) error {
	conn := meta.(*ovirtsdk4.Connection)
	vmsService := conn.SystemService().VmsService()

	cluster, err := ovirtsdk4.NewClusterBuilder().
		Id(d.Get("cluster_id").(string)).Build()
	if err != nil {
		return err
	}

	template, err := ovirtsdk4.NewTemplateBuilder().
		Name(d.Get("template").(string)).Build()
	if err != nil {
		return err
	}

	cpuTopo := ovirtsdk4.NewCpuTopologyBuilder().
		Cores(int64(d.Get("cores").(int))).
		Threads(int64(d.Get("threads").(int))).
		Sockets(int64(d.Get("sockets").(int))).
		MustBuild()

	cpu, err := ovirtsdk4.NewCpuBuilder().
		Topology(cpuTopo).
		Build()
	if err != nil {
		return err
	}

	initialBuilder := ovirtsdk4.NewInitializationBuilder().
		AuthorizedSshKeys(d.Get("authorized_ssh_key").(string))

	numNetworks := d.Get("network_interface.#").(int)
	ncBuilderSlice := make([]ovirtsdk4.NicConfigurationBuilder, numNetworks)
	for i := 0; i < numNetworks; i++ {
		prefix := fmt.Sprintf("network_interface.%d", i)

		ncBuilderSlice[i] = *ovirtsdk4.NewNicConfigurationBuilder().
			Name(d.Get(prefix + ".label").(string)).
			IpBuilder(
				ovirtsdk4.NewIpBuilder().
					Address(d.Get(prefix + ".ip_address").(string)).
					Netmask(d.Get(prefix + ".subnet_mask").(string)).
					Gateway(d.Get(prefix + ".gateway").(string))).
			BootProtocol(ovirtsdk4.BootProtocol(d.Get(prefix + ".boot_proto").(string))).
			OnBoot(d.Get(prefix + ".on_boot").(bool))
	}
	initialBuilder.NicConfigurationsBuilderOfAny(ncBuilderSlice...)

	initialize, err := initialBuilder.Build()
	if err != nil {
		return err
	}

	resp, err := vmsService.Add().
		Vm(
			ovirtsdk4.NewVmBuilder().
				Name(d.Get("name").(string)).
				Cluster(cluster).
				Template(template).
				Cpu(cpu).
				Initialization(initialize).
				MustBuild()).
		Send()

	if err != nil {
		return err
	}
	newVM, ok := resp.Vm()
	if !ok {
		d.SetId("")
		return nil
	}
	d.SetId(newVM.MustId())

	vmService := conn.SystemService().VmsService().VmService(d.Id())
	// Do attach disks
	err = buildOvirtVMDiskAttachment(newVM.MustId(), d, meta)
	if err != nil {
		return err
	}

	for i := 0; i < numNetworks; i++ {
		prefix := fmt.Sprintf("network_interface.%d", i)
		profilesService := conn.SystemService().VnicProfilesService()
		var profileID, pfDcID, newVMDcID string
		pfsResp, _ := profilesService.List().Send()
		pfSlice, _ := pfsResp.Profiles()
		for _, pf := range pfSlice.Slice() {
			pfNetwork, _ := conn.FollowLink(pf.MustNetwork())
			newVMCluster, _ := conn.FollowLink(newVM.MustCluster())
			if pfNetwork, ok := pfNetwork.(*ovirtsdk4.Network); ok {
				pfDcID = pfNetwork.MustDataCenter().MustId()
			}
			if newVMCluster, ok := newVMCluster.(*ovirtsdk4.Cluster); ok {
				newVMDcID = newVMCluster.MustDataCenter().MustId()
			}
			// this 'if' ensure this VnicProfile is exactly on this datacenter as you could
			// have multiple profiles on different datacenters with same name
			if (pfDcID == newVMDcID) && pf.MustName() == d.Get(prefix+".network").(string) {
				profileID = pf.MustId()
				break
			}
		}
		// Locate the service that manages the NICs of the virtual machine
		nicsService := vmsService.VmService(newVM.MustId()).NicsService()
		nicsService.Add().
			Nic(
				ovirtsdk4.NewNicBuilder().
					Name(fmt.Sprintf("nic%d", i+1)).
					Description(fmt.Sprintf("My network interface card #%d", i+1)).
					VnicProfile(
						ovirtsdk4.NewVnicProfileBuilder().
							Id(profileID).
							MustBuild()).
					MustBuild()).
			Send()
	}

	// Try to start VM
	_, err = vmService.Start().Send()
	if err != nil {
		return err
	}
	return resourceOvirtVMRead(d, meta)
}

func resourceOvirtVMUpdate(d *schema.ResourceData, meta interface{}) error {
	return nil
}

func resourceOvirtVMRead(d *schema.ResourceData, meta interface{}) error {
	conn := meta.(*ovirtsdk4.Connection)

	getVmresp, err := conn.SystemService().VmsService().
		VmService(d.Id()).Get().Send()
	if err != nil {
		if _, ok := err.(*ovirtsdk4.NotFoundError); ok {
			d.SetId("")
			return nil
		}
		return err
	}

	vm, ok := getVmresp.Vm()

	if !ok {
		d.SetId("")
		return nil
	}
	d.Set("name", vm.MustName())
	d.Set("cores", vm.MustCpu().MustTopology().MustCores())
	d.Set("sockets", vm.MustCpu().MustTopology().MustSockets())
	d.Set("threads", vm.MustCpu().MustTopology().MustThreads())
	d.Set("authorized_ssh_key", vm.MustInitialization().MustAuthorizedSshKeys())
	d.Set("cluster_id", vm.MustCluster().MustId())

	template, _ := conn.FollowLink(vm.MustTemplate())
	if template, ok := template.(*ovirtsdk4.Template); ok {
		d.Set("template", template.MustName())
	}

	if diskAttachments, ok := vm.DiskAttachments(); ok {
		d.Set("attached_disk", flattenOvirtVMDiskAttachment(diskAttachments.Slice()))
	}

	if initialization, ok := vm.Initialization(); ok {
		if nicConfigs, ok := initialization.NicConfigurations(); ok {
			d.Set("network_interface", flattenOvirtVMNetworkInterface(nicConfigs.Slice()))
		}
	}

	return nil
}

func resourceOvirtVMDelete(d *schema.ResourceData, meta interface{}) error {
	conn := meta.(*ovirtsdk4.Connection)

	vmService := conn.SystemService().VmsService().VmService(d.Id())

	return resource.Retry(3*time.Minute, func() *resource.RetryError {
		getVMResp, err := vmService.Get().Send()
		if err != nil {
			if _, ok := err.(*ovirtsdk4.NotFoundError); ok {
				return nil
			}
			return resource.RetryableError(err)
		}

		vm, ok := getVMResp.Vm()
		if !ok {
			d.SetId("")
			return nil
		}

		if vm.MustStatus() != ovirtsdk4.VMSTATUS_DOWN {
			_, err := vmService.Shutdown().Send()
			if err != nil {
				return resource.RetryableError(fmt.Errorf("Stop instance timeout and got an error: %v", err))
			}
		}
		//
		_, err = vmService.Remove().
			DetachOnly(true). // DetachOnly indicates without removing disks
			Send()
		if err != nil {
			return resource.RetryableError(fmt.Errorf("Delete instalce timeout and got an error: %v", err))
		}

		return nil

	})
}

func buildOvirtVMDiskAttachment(vmID string, d *schema.ResourceData, meta interface{}) error {
	conn := meta.(*ovirtsdk4.Connection)
	vmService := conn.SystemService().VmsService().VmService(vmID)
	attachmentSet := d.Get("attached_disk").(*schema.Set)
	for _, v := range attachmentSet.List() {
		attachment := v.(map[string]interface{})
		diskService := conn.SystemService().DisksService().
			DiskService(attachment["disk_id"].(string))
		var disk *ovirtsdk4.Disk
		err := resource.Retry(30*time.Second, func() *resource.RetryError {
			getDiskResp, err := diskService.Get().Send()
			if err != nil {
				return resource.RetryableError(err)
			}
			disk = getDiskResp.MustDisk()
			if disk.MustStatus() == ovirtsdk4.DISKSTATUS_LOCKED {
				return resource.RetryableError(fmt.Errorf("disk is locked, wait for next check"))
			}
			return nil
		})
		if err != nil {
			return err
		}
		_, err = vmService.DiskAttachmentsService().Add().
			Attachment(
				ovirtsdk4.NewDiskAttachmentBuilder().
					Disk(disk).
					Interface(ovirtsdk4.DiskInterface(attachment["interface"].(string))).
					Bootable(attachment["bootable"].(bool)).
					Active(attachment["active"].(bool)).
					LogicalName(attachment["logical_name"].(string)).
					PassDiscard(attachment["pass_discard"].(bool)).
					ReadOnly(attachment["read_only"].(bool)).
					UsesScsiReservation(attachment["use_scsi_reservation"].(bool)).
					MustBuild()).
			Send()
		if err != nil {
			return err
		}
	}
	return nil
}

func flattenOvirtVMDiskAttachment(configured []*ovirtsdk4.DiskAttachment) []map[string]interface{} {
	diskAttachments := make([]map[string]interface{}, len(configured))
	for _, v := range configured {
		attrs := make(map[string]interface{})
		attrs["disk_id"] = v.MustDisk().MustId()
		attrs["active"] = v.MustActive()
		attrs["bootable"] = v.MustBootable()
		attrs["interface"] = v.MustInterface()
		attrs["logical_name"] = v.MustLogicalName()
		attrs["pass_discard"] = v.MustPassDiscard()
		attrs["read_only"] = v.MustReadOnly()
		attrs["use_scsi_reservation"] = v.MustUsesScsiReservation()
		diskAttachments = append(diskAttachments, attrs)
	}
	return diskAttachments
}

func flattenOvirtVMNetworkInterface(configured []*ovirtsdk4.NicConfiguration) []map[string]interface{} {
	nicConfigs := make([]map[string]interface{}, len(configured))
	for _, v := range configured {
		attrs := make(map[string]interface{})
		if name, ok := v.Name(); ok {
			attrs["label"] = name
		}
		attrs["on_boot"] = v.MustOnBoot()
		attrs["boot_proto"] = v.MustBootProtocol()
		if ipAttrs, ok := v.Ip(); ok {
			if ipAddr, ok := ipAttrs.Address(); ok {
				attrs["ip_address"] = ipAddr
			}
			if netmask, ok := ipAttrs.Netmask(); ok {
				attrs["subnet_mask"] = netmask
			}
			if gateway, ok := ipAttrs.Gateway(); ok {
				attrs["gateway"] = gateway
			}
		}
		nicConfigs = append(nicConfigs, attrs)
	}
	return nicConfigs
}
