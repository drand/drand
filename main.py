import jinja2

template = jinja2.Environment(
    loader=jinja2.FileSystemLoader(searchpath="./")
).get_template("docker-compose.yaml.j2")

def make_container_list(latency, count):
    return [
        {
            "name": "drand_container_%s_%d" % (latency, i),
            "latency": latency,
            "volume_name": "drand_volume_%s_%d" % (latency, i)
        }
        for i in range(count)
    ]

hostlist = []
latencies_to_node_count = {
    '100ms': 1,
    '200ms': 2,
    '300ms': 3,
    '400ms': 4,
}

for l, c in latencies_to_node_count.items():
    hostlist = hostlist + make_container_list(l, c)

print(template.render(hosts=hostlist))
