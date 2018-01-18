package ovirt

import (
	"strconv"

	"github.com/EMSL-MSC/ovirtapi"
	"github.com/hashicorp/terraform/helper/schema"
)

func resourceDisk() *schema.Resource {
	return &schema.Resource{
		Create: resourceDiskCreate,
		Read:   resourceDiskRead,
		Delete: resourceDiskDelete,
		Schema: map[string]*schema.Schema{
			"name": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},
			"bootable": {
				Type:     schema.TypeBool,
				Optional: true,
				ForceNew: true,
			},
			"format": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},
			"storage_domain_id": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},
			"size": {
				Type:     schema.TypeInt,
				Required: true,
				ForceNew: true,
			},
			"shareable": {
				Type:     schema.TypeBool,
				Optional: true,
				ForceNew: true,
			},
			"sparse": {
				Type:     schema.TypeBool,
				Optional: true,
				ForceNew: true,
			},
		},
	}
}

func resourceDiskCreate(d *schema.ResourceData, meta interface{}) error {
	con := meta.(*ovirtapi.Connection)

	newDisk := con.NewDisk()
	err := resourceDiskModify(d, newDisk)
	if err != nil {
		newDisk.Delete()
		return err
	}
	d.SetId(newDisk.ID)
	return nil
}

func resourceDiskModify(d *schema.ResourceData, disk *ovirtapi.Disk) error {
	disk.ProvisionedSize = d.Get("size").(int)
	disk.Format = d.Get("format").(string)
	disk.Name = d.Get("name").(string)
	storageDomains := ovirtapi.StorageDomains{}
	storageDomains.StorageDomain = append(storageDomains.StorageDomain, ovirtapi.Link{
		ID: d.Get("storage_domain_id").(string),
	})
	disk.StorageDomains = &storageDomains
	if d.Get("bootable").(bool) {
		disk.Bootable = "false"
	}
	if d.Get("shareable").(bool) {
		disk.Shareable = "true"
	}
	if d.Get("sparse").(bool) {
		disk.Sparse = "true"
	}
	return disk.Save()
}

func resourceDiskRead(d *schema.ResourceData, meta interface{}) error {
	con := meta.(*ovirtapi.Connection)
	disk, err := con.GetDisk(d.Id())
	if err != nil {
		d.SetId("")
		return nil
	}

	d.Set("name", disk.Name)
	bootable, _ := strconv.ParseBool(disk.Bootable)
	d.Set("bootable", bootable)
	d.Set("size", disk.ProvisionedSize)
	d.Set("format", disk.Format)
	d.Set("storage_domain_id", disk.StorageDomains.StorageDomain[0].ID)
	shareable, _ := strconv.ParseBool(disk.Shareable)
	d.Set("shareable", shareable)
	sparse, _ := strconv.ParseBool(disk.Sparse)
	d.Set("sparse", sparse)
	return nil
}

func resourceDiskDelete(d *schema.ResourceData, meta interface{}) error {
	con := meta.(*ovirtapi.Connection)
	disk, err := con.GetDisk(d.Id())
	if err != nil {
		return nil
	}
	return disk.Delete()
}
