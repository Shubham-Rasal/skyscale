#!/usr/bin/env python3
"""
aws-gpu-spot — launch, use, and tear down AWS GPU instances (spot-first).

Credentials are read ONLY from the environment. This file contains NO secrets.
Required env: AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY
Optional env: AWS_DEFAULT_REGION (default us-east-1),
              GPU_SKILL_STATE_DIR (default ~/.cache/aws-gpu-spot)

Everything this script creates is tagged ManagedBy=aws-gpu-spot so it can find
and clean up after itself. ALWAYS terminate — GPU instances bill by the second.

Subcommands:
  whoami                     Verify creds; print account / IAM identity.
  list-gpus [--spot]         List GPU instance types (+ current spot prices).
  launch --name N ...        Launch a GPU instance (spot-first, AZ fallback).
  ssh-info --name N          Print ssh command for a launched instance.
  run --name N -- CMD...     Run a shell command on the instance over SSH.
  smoke --name N             Run a PyTorch/CUDA smoke test (PyTorch DLAMI).
  status                     List all ManagedBy=aws-gpu-spot instances.
  terminate --name N         Terminate one instance and delete its key + SG.
  cleanup-all                Terminate every managed instance and delete leftovers.
"""
import argparse, json, os, subprocess, sys, time, datetime

try:
    import boto3, botocore
except ImportError:
    sys.exit("boto3 is required:  pip install boto3")

TAG = "aws-gpu-spot"                      # ManagedBy tag value
REGION = os.environ.get("AWS_DEFAULT_REGION", "us-east-1")
STATE_DIR = os.path.expanduser(os.environ.get("GPU_SKILL_STATE_DIR", "~/.cache/aws-gpu-spot"))

# ---- knowledge baked in from hard-won experience ---------------------------
# Common single-GPU types (cheapest→bigger). Multi-GPU types can be passed too.
GPU_TYPES = ["g4dn.xlarge", "g6.xlarge", "g5.xlarge", "g6e.xlarge",
             "g7e.2xlarge", "p5.4xlarge"]
# AZ scan order for spot capacity (us-east-1). Spot capacity varies by AZ, so we
# sweep and take the first that launches. InsufficientInstanceCapacity => next AZ;
# MaxSpotInstanceCountExceeded => spot QUOTA too low (per-family, see SKILL.md).
DEFAULT_AZS = ["us-east-1a", "us-east-1b", "us-east-1d", "us-east-1f",
               "us-east-1c", "us-east-1e"]


def _require_creds():
    if not os.environ.get("AWS_ACCESS_KEY_ID") or not os.environ.get("AWS_SECRET_ACCESS_KEY"):
        sys.exit("ERROR: set AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY in the environment.")


def ec2():
    return boto3.client("ec2", region_name=REGION)


def _state_path(name):
    os.makedirs(STATE_DIR, exist_ok=True)
    return os.path.join(STATE_DIR, f"{name}.json")


def _save_state(name, st):
    json.dump(st, open(_state_path(name), "w"))


def _load_state(name):
    p = _state_path(name)
    if not os.path.exists(p):
        sys.exit(f"no launched instance named {name!r} (state file missing)")
    return json.load(open(p))


def _my_ip_cidr():
    """Public IP for the SSH security-group rule. Uses curl (handles TLS certs)."""
    try:
        ip = subprocess.check_output(["curl", "-s", "--max-time", "10",
                                      "https://checkip.amazonaws.com"], text=True).strip()
        if ip and ip.count(".") == 3:
            return ip + "/32"
    except Exception:
        pass
    sys.exit("could not detect public IP; pass --ssh-cidr x.x.x.x/32 explicitly")


def _latest_pytorch_dlami(c):
    imgs = c.describe_images(Owners=["amazon"], Filters=[
        {"Name": "name", "Values": ["Deep Learning OSS Nvidia Driver AMI GPU PyTorch*Ubuntu 22.04*"]},
        {"Name": "state", "Values": ["available"]},
        {"Name": "architecture", "Values": ["x86_64"]}])["Images"]
    if not imgs:
        sys.exit("no PyTorch DLAMI found; pass --ami explicitly")
    return sorted(imgs, key=lambda i: i["CreationDate"])[-1]["ImageId"]


def _default_subnets(c):
    subs = c.describe_subnets(Filters=[{"Name": "default-for-az", "Values": ["true"]}])["Subnets"]
    return {s["AvailabilityZone"]: s["SubnetId"] for s in subs}


def _ensure_key_and_sg(c, name, ssh_cidr):
    keyname = f"{TAG}-{name}"
    os.makedirs(STATE_DIR, exist_ok=True)
    pem = os.path.join(STATE_DIR, keyname + ".pem")
    # fresh key each launch
    try:
        c.delete_key_pair(KeyName=keyname)
    except botocore.exceptions.ClientError:
        pass
    if os.path.exists(pem):
        os.remove(pem)
    kp = c.create_key_pair(KeyName=keyname)
    open(pem, "w").write(kp["KeyMaterial"])
    os.chmod(pem, 0o600)
    # security group (delete any stale one of same name, then recreate)
    vpc = c.describe_vpcs(Filters=[{"Name": "isDefault", "Values": ["true"]}])["Vpcs"][0]["VpcId"]
    for g in c.describe_security_groups(Filters=[{"Name": "group-name", "Values": [keyname]}])["SecurityGroups"]:
        try:
            c.delete_security_group(GroupId=g["GroupId"])
        except botocore.exceptions.ClientError:
            pass
    sg = c.create_security_group(GroupName=keyname, Description="aws-gpu-spot ssh", VpcId=vpc)["GroupId"]
    c.authorize_security_group_ingress(GroupId=sg, IpPermissions=[
        {"IpProtocol": "tcp", "FromPort": 22, "ToPort": 22, "IpRanges": [{"CidrIp": ssh_cidr}]}])
    return keyname, pem, sg


def _tags(name):
    return [{"ResourceType": "instance", "Tags": [
        {"Key": "Name", "Value": f"{TAG}-{name}"},
        {"Key": "ManagedBy", "Value": TAG}]}]


# ---------------------------------------------------------------------------
def cmd_whoami(args):
    _require_creds()
    ident = boto3.client("sts", region_name=REGION).get_caller_identity()
    print(f"Account : {ident['Account']}")
    print(f"ARN     : {ident['Arn']}")
    print(f"Region  : {REGION}")


def cmd_list_gpus(args):
    _require_creds()
    c = ec2()
    types = []
    p = c.get_paginator("describe_instance_types")
    for page in p.paginate(Filters=[{"Name": "instance-type", "Values": ["g*", "p*"]}]):
        for it in page["InstanceTypes"]:
            gi = it.get("GpuInfo")
            if not gi:
                continue
            g = gi["Gpus"][0]
            types.append((it["InstanceType"], f"{g['Manufacturer']} {g['Name']}",
                          sum(x["Count"] for x in gi["Gpus"]),
                          gi["TotalGpuMemoryInMiB"] // 1024,
                          it["VCpuInfo"]["DefaultVCpus"]))
    types.sort()
    prices = {}
    if args.spot:
        now = datetime.datetime.now(datetime.timezone.utc)
        r = c.describe_spot_price_history(
            InstanceTypes=[t[0] for t in types], ProductDescriptions=["Linux/UNIX"],
            StartTime=now - datetime.timedelta(hours=1), EndTime=now)
        for pr in r["SpotPriceHistory"]:
            k = pr["InstanceType"]
            if k not in prices or float(pr["SpotPrice"]) < prices[k]:
                prices[k] = float(pr["SpotPrice"])
    print(f"{'type':<16}{'gpu':<24}{'#':>2}{'gpuGB':>7}{'vCPU':>6}" + ("  spot$/hr" if args.spot else ""))
    for t, gpu, n, mem, vcpu in types:
        line = f"{t:<16}{gpu:<24}{n:>2}{mem:>7}{vcpu:>6}"
        if args.spot and t in prices:
            line += f"  {prices[t]:>7.3f}"
        print(line)


def cmd_launch(args):
    _require_creds()
    c = ec2()
    ssh_cidr = args.ssh_cidr or _my_ip_cidr()
    ami = args.ami or _latest_pytorch_dlami(c)
    keyname, pem, sg = _ensure_key_and_sg(c, args.name, ssh_cidr)
    az_subnet = _default_subnets(c)
    azs = args.azs.split(",") if args.azs else DEFAULT_AZS

    def _spot_opts():
        return {"MarketType": "spot", "SpotOptions": {
            "SpotInstanceType": "one-time", "InstanceInterruptionBehavior": "terminate"}}

    def _try(az, market):
        kw = dict(ImageId=ami, InstanceType=args.type, MinCount=1, MaxCount=1,
                  KeyName=keyname, SecurityGroupIds=[sg], SubnetId=az_subnet[az],
                  BlockDeviceMappings=[{"DeviceName": "/dev/sda1", "Ebs": {
                      "VolumeSize": args.volume_size, "VolumeType": "gp3", "DeleteOnTermination": True}}],
                  TagSpecifications=_tags(args.name))
        if market == "spot":
            kw["InstanceMarketOptions"] = _spot_opts()
        return c.run_instances(**kw)["Instances"][0]["InstanceId"]

    order = {"spot": ["spot"], "on-demand": ["on-demand"],
             "spot-then-ondemand": ["spot", "on-demand"]}[args.market]
    iid = None
    for market in order:
        for az in azs:
            if az not in az_subnet:
                continue
            try:
                iid = _try(az, market)
                print(f"launched {market} {args.type} in {az} -> {iid}")
                market_used, az_used = market, az
                break
            except botocore.exceptions.ClientError as e:
                code = e.response["Error"]["Code"]
                print(f"  {market} {az}: {code}")
                if code == "MaxSpotInstanceCountExceeded":
                    print("  -> spot QUOTA too low for this family (see SKILL.md: quota L-3819A6DF for G/VT).")
                # InsufficientInstanceCapacity / capacity errors -> just try next AZ
        if iid:
            break
    if not iid:
        # nothing launched: clean the key + SG we created
        try: c.delete_security_group(GroupId=sg)
        except botocore.exceptions.ClientError: pass
        try: c.delete_key_pair(KeyName=keyname)
        except botocore.exceptions.ClientError: pass
        sys.exit("FAILED to launch in any AZ/market (capacity or quota). Cleaned up.")

    _save_state(args.name, {"iid": iid, "sg": sg, "key": keyname, "pem": pem,
                            "type": args.type, "az": az_used, "market": market_used, "ami": ami})
    print("waiting for running + public IP ...")
    c.get_waiter("instance_running").wait(InstanceIds=[iid])
    ip = c.describe_instances(InstanceIds=[iid])["Reservations"][0]["Instances"][0].get("PublicIpAddress")
    st = _load_state(args.name); st["ip"] = ip; _save_state(args.name, st)
    print(f"running: {iid}  ip={ip}")
    _wait_ssh(pem, ip)
    print(f"SSH ready.  ssh -i {pem} ubuntu@{ip}")


def _wait_ssh(pem, ip, tries=25):
    for i in range(tries):
        r = subprocess.run(["ssh", "-o", "StrictHostKeyChecking=no", "-o", "UserKnownHostsFile=/dev/null",
                            "-o", "ConnectTimeout=8", "-o", "BatchMode=yes", "-i", pem,
                            f"ubuntu@{ip}", "true"], capture_output=True)
        if r.returncode == 0:
            return True
        time.sleep(8)
    sys.exit("SSH did not become ready in time")


def _ssh(pem, ip, command):
    return subprocess.run(["ssh", "-o", "StrictHostKeyChecking=no", "-o", "UserKnownHostsFile=/dev/null",
                           "-o", "ConnectTimeout=15", "-i", pem, f"ubuntu@{ip}", command])


def cmd_ssh_info(args):
    st = _load_state(args.name)
    print(f"ssh -o StrictHostKeyChecking=no -i {st['pem']} ubuntu@{st.get('ip','<no-ip>')}")


def cmd_run(args):
    st = _load_state(args.name)
    sys.exit(_ssh(st["pem"], st["ip"], " ".join(args.command)).returncode)


def cmd_smoke(args):
    st = _load_state(args.name)
    script = (
        'nvidia-smi --query-gpu=name,driver_version,memory.total --format=csv,noheader; '
        'source /opt/pytorch/bin/activate 2>/dev/null || true; '
        'python - <<PY\n'
        'import torch, time\n'
        'print("torch", torch.__version__, "| CUDA", torch.version.cuda, "| avail", torch.cuda.is_available())\n'
        'assert torch.cuda.is_available()\n'
        'print("device:", torch.cuda.get_device_name(0))\n'
        'n=4096; a=torch.randn(n,n,device="cuda"); b=torch.randn(n,n,device="cuda")\n'
        'for _ in range(3): _=a@b\n'
        'torch.cuda.synchronize(); t=time.time()\n'
        'for _ in range(20): c=a@b\n'
        'torch.cuda.synchronize(); dt=(time.time()-t)/20\n'
        'print(f"matmul {n}^2: {dt*1e3:.1f} ms -> {2*n**3/dt/1e12:.2f} TFLOP/s fp32")\n'
        'err=(c[:2].cpu()-(a[:2].cpu()@b.cpu())).abs().max().item()\n'
        'print("max err vs CPU:", round(err,4)); print("SMOKE TEST PASSED" if err<1e-1 else "FAILED")\n'
        'PY'
    )
    sys.exit(_ssh(st["pem"], st["ip"], script).returncode)


def cmd_status(args):
    _require_creds()
    c = ec2()
    r = c.describe_instances(Filters=[{"Name": "tag:ManagedBy", "Values": [TAG]}])
    rows = [(i["InstanceId"], i["InstanceType"], i["State"]["Name"],
             i["Placement"]["AvailabilityZone"], i.get("PublicIpAddress", "-"))
            for res in r["Reservations"] for i in res["Instances"]]
    if not rows:
        print("no ManagedBy=aws-gpu-spot instances")
    for iid, t, state, az, ip in rows:
        print(f"{iid}  {t:<14} {state:<12} {az}  {ip}")


def _terminate_and_clean(c, iid, sg, key):
    c.terminate_instances(InstanceIds=[iid])
    print(f"terminating {iid} (billing stops at shutting-down) ...")
    # key can go immediately
    if key:
        try: c.delete_key_pair(KeyName=key); print("deleted key pair")
        except botocore.exceptions.ClientError: pass
    # SG can only be deleted after the ENI detaches (instance fully terminated)
    if sg:
        for _ in range(60):
            states = [i["State"]["Name"] for res in c.describe_instances(InstanceIds=[iid])["Reservations"]
                      for i in res["Instances"]]
            if states and all(s == "terminated" for s in states):
                try: c.delete_security_group(GroupId=sg); print("deleted security group")
                except botocore.exceptions.ClientError as e: print("SG delete:", e.response["Error"]["Code"])
                break
            time.sleep(10)


def cmd_terminate(args):
    _require_creds()
    st = _load_state(args.name)
    _terminate_and_clean(ec2(), st["iid"], st.get("sg"), st.get("key"))
    for f in (st.get("pem"), _state_path(args.name)):
        if f and os.path.exists(f):
            os.remove(f)
    print("done — nothing left billing")


def cmd_cleanup_all(args):
    _require_creds()
    c = ec2()
    r = c.describe_instances(Filters=[
        {"Name": "tag:ManagedBy", "Values": [TAG]},
        {"Name": "instance-state-name", "Values": ["pending", "running", "stopping", "stopped"]}])
    iids = [i["InstanceId"] for res in r["Reservations"] for i in res["Instances"]]
    if iids:
        c.terminate_instances(InstanceIds=iids)
        print("terminating:", iids)
        c.get_waiter("instance_terminated").wait(InstanceIds=iids)
    # delete all managed SGs + keys
    for g in c.describe_security_groups(Filters=[{"Name": "group-name", "Values": [f"{TAG}-*"]}])["SecurityGroups"]:
        try: c.delete_security_group(GroupId=g["GroupId"]); print("deleted SG", g["GroupName"])
        except botocore.exceptions.ClientError as e: print("SG:", e.response["Error"]["Code"])
    for k in c.describe_key_pairs(Filters=[{"Name": "key-name", "Values": [f"{TAG}-*"]}])["KeyPairs"]:
        try: c.delete_key_pair(KeyName=k["KeyName"]); print("deleted key", k["KeyName"])
        except botocore.exceptions.ClientError: pass
    print("cleanup-all done")


def main():
    ap = argparse.ArgumentParser(prog="gpu.py", description="AWS GPU spot launcher (creds from env)")
    sub = ap.add_subparsers(dest="cmd", required=True)
    sub.add_parser("whoami")
    g = sub.add_parser("list-gpus"); g.add_argument("--spot", action="store_true")
    l = sub.add_parser("launch")
    l.add_argument("--name", default="gpu")
    l.add_argument("--type", default="g4dn.xlarge")
    l.add_argument("--market", default="spot-then-ondemand",
                   choices=["spot", "on-demand", "spot-then-ondemand"])
    l.add_argument("--ami", default=None, help="default: latest PyTorch DLAMI")
    l.add_argument("--volume-size", type=int, default=120)
    l.add_argument("--azs", default=None, help="comma list; default sweeps all us-east-1 AZs")
    l.add_argument("--ssh-cidr", default=None, help="default: your detected public IP /32")
    s = sub.add_parser("ssh-info"); s.add_argument("--name", default="gpu")
    rp = sub.add_parser("run"); rp.add_argument("--name", default="gpu"); rp.add_argument("command", nargs=argparse.REMAINDER)
    sm = sub.add_parser("smoke"); sm.add_argument("--name", default="gpu")
    sub.add_parser("status")
    tp = sub.add_parser("terminate"); tp.add_argument("--name", default="gpu")
    sub.add_parser("cleanup-all")
    args = ap.parse_args()
    {"whoami": cmd_whoami, "list-gpus": cmd_list_gpus, "launch": cmd_launch,
     "ssh-info": cmd_ssh_info, "run": cmd_run, "smoke": cmd_smoke, "status": cmd_status,
     "terminate": cmd_terminate, "cleanup-all": cmd_cleanup_all}[args.cmd](args)


if __name__ == "__main__":
    main()
