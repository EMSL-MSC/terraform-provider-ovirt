package ovirt

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform/helper/resource"
	"github.com/hashicorp/terraform/terraform"
	ovirtsdk4 "gopkg.in/imjoey/go-ovirt.v4"
)

func TestAccOvirtVM_basic(t *testing.T) {
	var vm ovirtsdk4.Vm
	resource.Test(t, resource.TestCase{
		PreCheck:      func() { testAccPreCheck(t) },
		Providers:     testAccProviders,
		IDRefreshName: "ovirt_vm.vm",
		CheckDestroy:  testAccCheckVMDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccVMBasic,
				Check: resource.ComposeTestCheckFunc(
					testAccCheckOvirtVMExists("ovirt_vm.vm", &vm),
					resource.TestCheckResourceAttr("ovirt_vm.vm", "name", "testAccOvirtVMBasic"),
					resource.TestCheckResourceAttr("ovirt_vm.vm", "network_interface.#", "1"),
					resource.TestCheckResourceAttr("ovirt_vm.vm", "attached_disk.#", "1"),
				),
			},
		},
	})
}

func testAccCheckVMDestroy(s *terraform.State) error {
	conn := testAccProvider.Meta().(*ovirtsdk4.Connection)
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "ovirt_vm" {
			continue
		}
		getResp, err := conn.SystemService().VmsService().
			VmService(rs.Primary.ID).
			Get().
			Send()
		if err != nil {
			if _, ok := err.(*ovirtsdk4.NotFoundError); ok {
				continue
			}
			return err
		}
		if _, ok := getResp.Vm(); ok {
			return fmt.Errorf("VM %s still exist", rs.Primary.ID)
		}
	}
	return nil
}

func testAccCheckOvirtVMExists(n string, v *ovirtsdk4.Vm) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[n]
		if !ok {
			return fmt.Errorf("Not found: %s", n)
		}
		if rs.Primary.ID == "" {
			return fmt.Errorf("No VM ID is set")
		}
		conn := testAccProvider.Meta().(*ovirtsdk4.Connection)
		getResp, err := conn.SystemService().VmsService().
			VmService(rs.Primary.ID).
			Get().
			Send()
		if err != nil {
			return err
		}
		vm, ok := getResp.Vm()
		if ok {
			*v = *vm
			return nil
		}
		return fmt.Errorf("VM %s not exist", rs.Primary.ID)
	}
}

const testAccVMBasic = `
resource "ovirt_vm" "vm" {
	name        = "testAccOvirtVMBasic"
	cluster_id  = "${data.ovirt_clusters.default.clusters.0.id}"
	network_interface {
		network     = "${data.ovirt_networks.ovirtmgmt.networks.0.name}"
		label       = "eth0"
		boot_proto  = "static"
		ip_address  = "10.1.60.60"
		gateway     = "10.1.60.1"
		subnet_mask = "255.255.255.0"
	}
	attached_disk {
		disk_id = "${ovirt_disk.vm_disk.id}"
		bootable = true
		interface = "virtio"
	}
}

resource "ovirt_disk" "vm_disk" {
	name              = "vm_disk"
	alias             = "vm_disk"
	size              = 23687091200
	format            = "cow"
	storage_domain_id = "${data.ovirt_storagedomains.data.storagedomains.0.id}"
	sparse            = true
}

data "ovirt_storagedomains" "data" {
	name_regex = "^data"
	search = {
	  criteria       = "name = data and datacenter = ${data.ovirt_datacenters.default.datacenters.0.name}"
	  case_sensitive = false
	}
}

data "ovirt_datacenters" "default" {
	search = {
		criteria       = "name = Default"
		max            = 1
		case_sensitive = false
	}
}

data "ovirt_clusters" "default" {
	search = {
		criteria       = "name = Default"
		max            = 1
		case_sensitive = false
	}
}

data "ovirt_networks" "ovirtmgmt" {
	search = {
	  criteria       = "datacenter = Default and name = ovirtmgmt"
	  max            = 1
	  case_sensitive = false
	}
}

`
