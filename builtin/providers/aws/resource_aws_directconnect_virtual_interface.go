package aws

import (
	"fmt"
	"log"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/ec2"
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
							Rquired: true,
						},
					},
				},
			},
		},
	}
}

func resourceAwsDirectconnectVirtualInterfaceCreate(d *schema.ResourceData, meta interface{}) error {
	conn := meta.(*AWSClient).ec2conn
	connID := aws.String(d.Get("connection_id").(string))

	// Create the directconnect virtual interface
	createInterfaceOpts := &ec2.NewPrivateVirtualInterface{
		VirtualInterfaceName: aws.String(d.Get("virtual_interface_name").(string)),
		Vlan:                 aws.String(d.Get("vlan").(string)),
		Asn:                  aws.String(d.Get("asn").(string)),
		AuthKey:              aws.String(d.Get("auth_key").(string)),
		AmazonAddress:        aws.String(d.Get("amazon_address").(string)),
		CustomerAddress:      aws.String(d.Get("customer_address").(string)),
		VirtualGatewayId:     aws.String(d.Get("virtual_gateway_id").(string)),
	}
	createOpts := &ec2.CreatePrivateVirtualInterfaceInput{
		ConnectionId:               connID,
		NewPrivateVirtualInterface: createInterfaceOpts,
	}
	log.Printf("[DEBUG] DiretconnectVirtualInterfaceCreate create config: %#v", createInterfaceOpts)
	resp, err := conn.CreatePrivateVirtualInterface(createOpts)
	if err != nil {
		return fmt.Errorf("Error creating direct connect virtual interface: %s", err)
	}

	// Get the ID and store it
	VirtualInterface := resp.VirtualInterface
	d.SetId(*VirtualInterface.virtualInterfaceId)
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

func resourceAwsVPCPeeringRead(d *schema.ResourceData, meta interface{}) error {
	conn := meta.(*AWSClient).ec2conn
	pcRaw, _, err := resourceAwsVPCPeeringConnectionStateRefreshFunc(conn, d.Id())()
	if err != nil {
		return err
	}
	if pcRaw == nil {
		d.SetId("")
		return nil
	}

	pc := pcRaw.(*ec2.VpcPeeringConnection)

	// The failed status is a status that we can assume just means the
	// connection is gone. Destruction isn't allowed, and it eventually
	// just "falls off" the console. See GH-2322
	if pc.Status != nil {
		if *pc.Status.Code == "failed" || *pc.Status.Code == "deleted" {
			log.Printf("[DEBUG] VPC Peering Connect (%s) in state (%s), removing", d.Id(), *pc.Status.Code)
			d.SetId("")
			return nil
		}
	}

	d.Set("accept_status", *pc.Status.Code)
	d.Set("peer_owner_id", pc.AccepterVpcInfo.OwnerId)
	d.Set("peer_vpc_id", pc.AccepterVpcInfo.VpcId)
	d.Set("vpc_id", pc.RequesterVpcInfo.VpcId)
	d.Set("tags", tagsToMap(pc.Tags))

	return nil
}

func resourceVPCPeeringConnectionAccept(conn *ec2.EC2, id string) (string, error) {

	log.Printf("[INFO] Accept VPC Peering Connection with id: %s", id)

	req := &ec2.AcceptVpcPeeringConnectionInput{
		VpcPeeringConnectionId: aws.String(id),
	}

	resp, err := conn.AcceptVpcPeeringConnection(req)
	if err != nil {
		return "", err
	}
	pc := resp.VpcPeeringConnection
	return *pc.Status.Code, err
}

func resourceAwsVPCPeeringUpdate(d *schema.ResourceData, meta interface{}) error {
	conn := meta.(*AWSClient).ec2conn

	if err := setTags(conn, d); err != nil {
		return err
	} else {
		d.SetPartial("tags")
	}
d.GetOk(key string)
	if _, ok := d.GetOk("auto_accept"); ok {
		pcRaw, _, err := resourceAwsVPCPeeringConnectionStateRefreshFunc(conn, d.Id())()

		if err != nil {
			return err
		}
		if pcRaw == nil {
			d.SetId("")
			return nil
		}
		pc := pcRaw.(*ec2.VpcPeeringConnection)

		if pc.Status != nil && *pc.Status.Code == "pending-acceptance" {
			status, err := resourceVPCPeeringConnectionAccept(conn, d.Id())
			if err != nil {
				return err
			}
			log.Printf(
				"[DEBUG] VPC Peering connection accept status: %s",
				status)
		}
	}

	return resourceAwsVPCPeeringRead(d, meta)
}

func resourceAwsVPCPeeringDelete(d *schema.ResourceData, meta interface{}) error {
	conn := meta.(*AWSClient).ec2conn

	_, err := conn.DeleteVpcPeeringConnection(
		&ec2.DeleteVpcPeeringConnectionInput{
			VpcPeeringConnectionId: aws.String(d.Id()),
		})
	return err
}

// resourceAwsVPCPeeringConnectionStateRefreshFunc returns a resource.StateRefreshFunc that is used to watch
// a VPCPeeringConnection.
func resourceAwsDirectconnectVirtualInterfaceStateRefreshFunc(conn *ec2.EC2, id string, connid string) resource.StateRefreshFunc {
	return func() (interface{}, string, error) {

		resp, err := conn.DescribeVirtualInterfaces(&ec2.DescribeVirtualInterfacesInput{
			connectionId:       []*string{aws.String(connid)},
			virtualInterfaceId: []*string{aws.String(id)},
		})
		if err != nil {
			if ec2err, ok := err.(awserr.Error); ok && ec2err.Code() == "InvalidVpcPeeringConnectionID.NotFound" {
				resp = nil
			} else {
				log.Printf("Error on VPCPeeringConnectionStateRefresh: %s", err)
				return nil, "", err
			}
		}

		if resp == nil {
			// Sometimes AWS just has consistency issues and doesn't see
			// our instance yet. Return an empty state.
			return nil, "", nil
		}

		pc := resp.VpcPeeringConnections[0]

		return pc, *pc.Status.Code, nil
	}
}
