package aws

import (
	"fmt"
	"log"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/directconnect"
	"github.com/hashicorp/terraform/helper/resource"
	"github.com/hashicorp/terraform/helper/schema"
)

func resourceAwsDirectconnectVirtualInterface() *schema.Resource {
	return &schema.Resource{
		Create: resourceAwsDirectconnectVirtualInterfaceCreate,
		Read:   resourceAwsDirectconnectVirtualInterfaceRead,
		Update: resourceAwsDirectconnectVirtualInterfaceUpdate,
		Delete: resourceAwsDirectconnectVirtualInterfaceDelete,

		Schema: map[string]*schema.Schema{
			"connection_id": &schema.Schema{
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},
			"virtual_interface_name": &schema.Schema{
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},
			"vlan": &schema.Schema{
				Type:     schema.TypeInt,
				Required: true,
				ForceNew: true,
			},
			"asn": &schema.Schema{
				Type:     schema.TypeInt,
				Required: true,
				ForceNew: true,
			},
			"auth_key": &schema.Schema{
				Type:     schema.TypeString,
				Required: false,
				ForceNew: true,
			},
			"amazon_address": &schema.Schema{
				Type:     schema.TypeString,
				Required: false,
				ForceNew: true,
			},
			"customer_address": &schema.Schema{
				Type:     schema.TypeString,
				Required: false,
				ForceNew: true,
			},
			"virtual_gateway_id": &schema.Schema{
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},
			"route_filter_prefixes": &schema.Schema{
				Type:     schema.TypeList,
				Optional: true,
				ForceNew: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"cidr": &schema.Schema{
							Type:    schema.TypeString,
							Required: true,
						},
					},
				},
			},
		},
	}
}

func resourceAwsDirectconnectVirtualInterfaceCreate(d *schema.ResourceData, meta interface{}) error {
	conn := meta.(*AWSClient).dirconn
	connID := aws.String(d.Get("connection_id").(string))

	// Create the directconnect virtual interface
	createInterfaceOpts := &directconnect.NewPrivateVirtualInterface{
		VirtualInterfaceName: aws.String(d.Get("virtual_interface_name").(string)),
		Vlan:                 aws.String(d.Get("vlan").(string)),
		Asn:                  aws.String(d.Get("asn").(string)),
		AuthKey:              aws.String(d.Get("auth_key").(string)),
		AmazonAddress:        aws.String(d.Get("amazon_address").(string)),
		CustomerAddress:      aws.String(d.Get("customer_address").(string)),
		VirtualGatewayId:     aws.String(d.Get("virtual_gateway_id").(string)),
	}
	createOpts := &directconnect.CreatePrivateVirtualInterfaceInput{
		ConnectionId:               connID,
		NewPrivateVirtualInterface: createInterfaceOpts,
	}
	log.Printf("[DEBUG] DiretconnectVirtualInterfaceCreate create config: %#v", createInterfaceOpts)
	resp, err := conn.CreatePrivateVirtualInterface(createOpts)
	if err != nil {
		return fmt.Errorf("Error creating direct connect virtual interface: %s", err)
	}

	// Get the ID and store it
	d.SetId(resp.VirtualInterfaceId)
	log.Printf("[INFO] Direct Connect private virtual interface ID: %s", d.Id())

	// Wait for the direct connect virtual interface to become available
	log.Printf(
		"[DEBUG] Waiting for direct connect virtual interface (%s) to become available",
		d.Id())
	stateConf := &resource.StateChangeConf{
		Pending: []string{"pending"},
		Target:  []string{"confirming"},
		Refresh: resourceAwsDirectconnectVirtualInterfaceStateRefreshFunc(conn, connID, d.Id()),
		Timeout: 1 * time.Minute,
	}
	if _, err := stateConf.WaitForState(); err != nil {
		return fmt.Errorf(
			"Error waiting for direct connect virtual interface (%s) to become available: %s",
			d.Id(), err)
	}

	return resourceAwsVPCPeeringUpdate(d, meta)
}

func resourceAwsDirectconnectVirtualInterfaceRead(d *schema.ResourceData, meta interface{}) error {
	conn := meta.(*AWSClient).dirconn
	connID := aws.String(d.Get("connection_id").(string))

	viRaw, _, err := resourceAwsDirectconnectVirtualInterfaceStateRefreshFunc(conn, d.Id(), connID)()
	if err != nil {
		return err
	}
	if viRaw == nil {
		d.SetId("")
		return nil
	}

	vi := viRaw.(*directconnect.VirtualInterface)

	d.Set("connectionId", *vi.ConnectionId)
	d.Set("virtual_interface_name", vi.VirtualInterfaceName)
	d.Set("vlan", vi.Vlan)
	d.Set("asn", vi.Asn)
	d.Set("auth_key", vi.AuthKey)
	d.Set("amazon_address", vi.AmazonAddress)
	d.Set("customer_address", vi.CustomerAddress)
	d.Set("virtual_gateway_id", vi.VirtualGatewayId)
	d.Set("route_filter_prefixes", vi.RouteFilterPrefixes)

	return nil
}

func resourceAwsDirectconnectVirtualInterfaceUpdate(d *schema.ResourceData, meta interface{}) error {
	conn := meta.(*AWSClient).dirconn
	connID := aws.String(d.Get("connection_id").(string))

	viRaw, _, err := resourceAwsDirectconnectVirtualInterfaceStateRefreshFunc(conn, d.Id(), connID)()

	if err != nil {
		return err
	}
	if viRaw == nil {
		d.SetId("")
		return nil
	}

	return resourceAwsDirectconnectVirtualInterfaceRead(d, meta)
}

func resourceAwsDirectconnectVirtualInterfaceDelete(d *schema.ResourceData, meta interface{}) error {
	conn := meta.(*AWSClient).dirconn

	_, err := conn.DeleteVirtualInterface(
		&directconnect.DeleteVirtualInterfaceInput{
			VirtualInterfaceId: aws.String(d.Id()),
		})
	return err
}

// resourceAwsVPCPeeringConnectionStateRefreshFunc returns a resource.StateRefreshFunc that is used to watch
// a VPCPeeringConnection.
func resourceAwsDirectconnectVirtualInterfaceStateRefreshFunc(conn *directconnect.DirectConnect, id string, connId string) resource.StateRefreshFunc {
	return func() (interface{}, string, error) {
		resp, err := conn.DescribeVirtualInterfaces(&directconnect.DescribeVirtualInterfacesInput{
			ConnectionId:       []*string{aws.String(connId)},
			VirtualInterfaceId: []*string{aws.String(id)},
		})
		if err != nil {
			if ec2err, ok := err.(awserr.Error); ok && ec2err.Code() == "InvalidDirectconnectVirtualInterfaceID.NotFound" {
				resp = nil
			} else {
				log.Printf("Error on DirectConnectVirtualInterfaceStateRefresh: %s", err)
				return nil, "", err
			}
		}

		if resp == nil {
			// Sometimes AWS just has consistency issues and doesn't see
			// our instance yet. Return an empty state.
			return nil, "", nil
		}

		vi := resp.VirtualInterfaces[0]

		return vi, *vi.VirtualInterfaceId, nil
	}
}
