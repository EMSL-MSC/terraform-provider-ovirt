package ovirt

import (
	"log"
	"strconv"
	"time"

	"github.com/EMSL-MSC/ovirtapi"
	"github.com/hashicorp/terraform/helper/schema"
)

func resourceVM() *schema.Resource {
	return &schema.Resource{
		Create: resourceVMCreate,
		Read:   resourceVMRead,
		Update: resourceVMUpdate,
		Delete: resourceVMDelete,
		Schema: map[string]*schema.Schema{
			"name": {
				Type:     schema.TypeString,
				Required: true,
			},
			"cluster": {
				Type:     schema.TypeString,
				Optional: true,
				Default:  "Default",
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
			"network_interface": {
				Type:     schema.TypeSet,
				Required: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"label": &schema.Schema{
							Type:     schema.TypeString,
							Optional: true,
						},

						"boot_proto": &schema.Schema{
							Type:     schema.TypeString,
							Optional: true,
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
						},
					},
				},
			},
			"attached_disks": {
				Type:     schema.TypeSet,
				Optional: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"attach_id": &schema.Schema{
							Type:     schema.TypeString,
							Computed: true,
						},
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
		},
	}
}

func resourceVMCreate(d *schema.ResourceData, meta interface{}) error {
	con := meta.(*ovirtapi.Connection)
	newVM := con.NewVM()
	newVM.Name = d.Get("name").(string)

	cluster := con.NewCluster()
	cluster.Name = d.Get("cluster").(string)
	newVM.Cluster = cluster

	template := con.NewTemplate()
	template.Name = d.Get("template").(string)
	newVM.Template = template
	newVM.CPU = &ovirtapi.CPU{
		Topology: &ovirtapi.CPUTopology{
			Cores:   d.Get("cores").(int),
			Sockets: d.Get("sockets").(int),
			Threads: d.Get("threads").(int),
		},
	}
	newVM.Initialization = &ovirtapi.Initialization{}

	newVM.Initialization.AuthorizedSSHKeys = d.Get("authorized_ssh_key").(string)

	numNetworks := d.Get("network_interface.#").(int)
	NICConfigurations := make([]ovirtapi.NICConfiguration, numNetworks)
	networkInterfaces := d.Get("network_interface").(*schema.Set)
	for i, v := range networkInterfaces.List() {
		networkInterface := v.(map[string]interface{})
		NICConfigurations[i] = ovirtapi.NICConfiguration{
			IP: &ovirtapi.IP{
				Address: networkInterface["ip_address"].(string),
				Netmask: networkInterface["subnet_mask"].(string),
				Gateway: networkInterface["gateway"].(string),
			},
			BootProtocol: networkInterface["boot_proto"].(string),
			OnBoot:       strconv.FormatBool(networkInterface["on_boot"].(bool)),
			Name:         networkInterface["label"].(string),
		}
		if i == 0 {
			d.SetConnInfo(map[string]string{
				"host": networkInterface["ip_address"].(string),
			})
		}
	}
	newVM.Initialization.NICConfigurations = &ovirtapi.NICConfigurations{NICConfiguration: NICConfigurations}

	err := newVM.Save()
	if err != nil {
		return err
	}
	d.SetId(newVM.ID)

	for newVM.Status != "down" {
		time.Sleep(time.Second)
		newVM.Update()
	}

	attachmentSet := d.Get("attached_disks").(*schema.Set)
	for _, v := range attachmentSet.List() {
		attachment := v.(map[string]interface{})
		disk, err := con.GetDisk(attachment["disk_id"].(string))
		if err != nil {
			return err
		}
		diskAttachment := ovirtapi.DiskAttachment{
			Disk:                disk,
			Active:              strconv.FormatBool(attachment["active"].(bool)),
			Bootable:            strconv.FormatBool(attachment["bootable"].(bool)),
			Interface:           attachment["interface"].(string),
			LogicalName:         attachment["logical_name"].(string),
			PassDiscard:         strconv.FormatBool(attachment["pass_discard"].(bool)),
			ReadOnly:            strconv.FormatBool(attachment["read_only"].(bool)),
			UsesSCSIReservation: strconv.FormatBool(attachment["use_scsi_reservation"].(bool)),
		}
		attachmentID, err := newVM.AddLinkObject("diskattachments", diskAttachment, nil)
		v.(*schema.ResourceData).Set("attach_id", attachmentID)
		if err != nil {
			return err
		}
		log.Printf("Attachment ID: (%s)\n", attachmentID)
		log.Printf("%v\n", attachment)
	}

	err = newVM.Start("", "", "", "true", "", nil)
	if err != nil {
		newVM.Delete()
		return err
	}
	return nil
}

func resourceVMUpdate(d *schema.ResourceData, meta interface{}) error {
	return nil
}
func resourceVMRead(d *schema.ResourceData, meta interface{}) error {
	con := meta.(*ovirtapi.Connection)
	vm, err := con.GetVM(d.Id())

	if err != nil {
		d.SetId("")
		return nil
	}
	d.Set("name", vm.Name)

	cluster, err := con.GetCluster(vm.Cluster.ID)
	if err != nil {
		d.SetId("")
		return nil
	}
	d.Set("cluster", cluster.Name)

	template, err := con.GetTemplate(vm.Template.ID)
	if err != nil {
		d.SetId("")
		return nil
	}
	d.Set("template", template.Name)
	d.Set("cores", vm.CPU.Topology.Cores)
	d.Set("sockets", vm.CPU.Topology.Sockets)
	d.Set("threads", vm.CPU.Topology.Threads)
	d.Set("authorized_ssh_key", vm.Initialization.AuthorizedSSHKeys)

	// for _, v := range d.Get("attached_disks").(*schema.Set).List() {
	// 	attachment := v(map[string]interface{})
	// }

	return nil
}

func resourceVMDelete(d *schema.ResourceData, meta interface{}) error {
	con := meta.(*ovirtapi.Connection)
	vm, err := con.GetVM(d.Id())
	if err != nil {
		return nil
	}
	if vm.Status != "down" {
		vm.Stop("false")
	}
	for vm.Status != "down" {
		time.Sleep(time.Second)
		vm.Update()
	}
	return vm.Delete()
}
