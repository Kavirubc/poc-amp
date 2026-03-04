// go:build ignore

// HTTP Traffic Capture eBPF Program
// Captures HTTP requests/responses from agent containers

#include "vmlinux.h"
#include <bpf/bpf_endian.h>
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_tracing.h>

#define ETH_P_IP 0x0800
#define IPPROTO_TCP 6

#define HTTP_GET 0x20544547  // "GET "
#define HTTP_POST 0x54534F50 // "POST"
#define HTTP_PUT 0x20545550  // "PUT "
#define HTTP_DEL 0x454C4544  // "DELE"
#define HTTP_RESP 0x50545448 // "HTTP"

#define MAX_HTTP_DATA 1024
#define MAX_CONTAINERS 256

// Event types
#define EVENT_HTTP_REQUEST 1
#define EVENT_HTTP_RESPONSE 2

// HTTP event structure sent to userspace
struct http_event {
  __u32 event_type;
  __u32 src_ip;
  __u32 dst_ip;
  __u16 src_port;
  __u16 dst_port;
  __u32 pid;
  __u32 data_len;
  __u64 timestamp;
  __u32 ifindex;
  char data[MAX_HTTP_DATA];
};

// Map to track which cgroup IDs belong to AMP containers
struct {
  __uint(type, BPF_MAP_TYPE_HASH);
  __uint(max_entries, MAX_CONTAINERS);
  __type(key, __u64);   // cgroup_id
  __type(value, __u32); // 1 = tracked
} amp_containers SEC(".maps");

// Map to track container info by cgroup ID
struct container_info {
  char agent_id[64];
  __u32 tracked;
};

struct {
  __uint(type, BPF_MAP_TYPE_HASH);
  __uint(max_entries, MAX_CONTAINERS);
  __type(key, __u64);
  __type(value, struct container_info);
} container_map SEC(".maps");

// Perf event buffer for sending events to userspace
struct {
  __uint(type, BPF_MAP_TYPE_PERF_EVENT_ARRAY);
  __uint(key_size, sizeof(__u32));
  __uint(value_size, sizeof(__u32));
} events SEC(".maps");

// Ring buffer alternative (for newer kernels)
struct {
  __uint(type, BPF_MAP_TYPE_RINGBUF);
  __uint(max_entries, 256 * 1024);
} rb SEC(".maps");

// Check if this is an HTTP method
static __always_inline int is_http_request(__u32 first_word) {
  return (first_word == HTTP_GET || first_word == HTTP_POST ||
          first_word == HTTP_PUT || first_word == HTTP_DEL);
}

static __always_inline int is_http_response(__u32 first_word) {
  return first_word == HTTP_RESP;
}

// TC ingress/egress hook for capturing HTTP traffic
SEC("tc")
int capture_http(struct __sk_buff *skb) {
  void *data = (void *)(long)skb->data;
  void *data_end = (void *)(long)skb->data_end;

  // Parse Ethernet header
  struct ethhdr *eth = data;
  if ((void *)(eth + 1) > data_end)
    return TC_ACT_OK;

  // Only process IPv4
  if (eth->h_proto != bpf_htons(ETH_P_IP))
    return TC_ACT_OK;

  // Parse IP header
  struct iphdr *ip = (void *)(eth + 1);
  if ((void *)(ip + 1) > data_end)
    return TC_ACT_OK;

  // Only process TCP
  if (ip->protocol != IPPROTO_TCP)
    return TC_ACT_OK;

  // Calculate IP header length
  __u32 ip_hdr_len = ip->ihl * 4;
  if (ip_hdr_len < sizeof(struct iphdr))
    return TC_ACT_OK;

  // Parse TCP header
  struct tcphdr *tcp = (void *)ip + ip_hdr_len;
  if ((void *)(tcp + 1) > data_end)
    return TC_ACT_OK;

  // Calculate TCP header length
  __u32 tcp_hdr_len = tcp->doff * 4;
  if (tcp_hdr_len < sizeof(struct tcphdr))
    return TC_ACT_OK;

  // Get HTTP payload
  void *http_data = (void *)tcp + tcp_hdr_len;
  if (http_data + 4 > data_end)
    return TC_ACT_OK;

  // Check for HTTP
  __u32 first_word;
  bpf_probe_read_kernel(&first_word, sizeof(first_word), http_data);

  __u32 event_type = 0;
  if (is_http_request(first_word)) {
    event_type = EVENT_HTTP_REQUEST;
  } else if (is_http_response(first_word)) {
    event_type = EVENT_HTTP_RESPONSE;
  }

  if (event_type == 0)
    return TC_ACT_OK;

  // Calculate payload length
  __u32 payload_len = data_end - http_data;
  if (payload_len > MAX_HTTP_DATA)
    payload_len = MAX_HTTP_DATA;

  // Prepare event
  struct http_event *event;
  event = bpf_ringbuf_reserve(&rb, sizeof(*event), 0);
  if (!event)
    return TC_ACT_OK;

  event->event_type = event_type;
  event->src_ip = ip->saddr;
  event->dst_ip = ip->daddr;
  event->src_port = bpf_ntohs(tcp->source);
  event->dst_port = bpf_ntohs(tcp->dest);
  event->timestamp = bpf_ktime_get_ns();
  event->ifindex = skb->ifindex;
  event->data_len = payload_len;
  event->pid = 0; // Will be filled by socket hooks

  // Copy HTTP data
  if (payload_len > 0) {
    bpf_probe_read_kernel(event->data, payload_len, http_data);
  }

  bpf_ringbuf_submit(event, 0);

  return TC_ACT_OK;
}

// Socket-level hook to capture with PID context
SEC("kprobe/tcp_sendmsg")
int trace_tcp_sendmsg(struct pt_regs *ctx) {
  __u64 cgroup_id = bpf_get_current_cgroup_id();

  // Check if this cgroup is tracked
  __u32 *tracked = bpf_map_lookup_elem(&amp_containers, &cgroup_id);
  if (!tracked || *tracked != 1)
    return 0;

  struct sock *sk = (struct sock *)PT_REGS_PARM1(ctx);
  struct msghdr *msg = (struct msghdr *)PT_REGS_PARM2(ctx);
  size_t size = PT_REGS_PARM3(ctx);

  if (size < 4 || size > MAX_HTTP_DATA)
    return 0;

  // Get socket info
  __u16 family = 0;
  bpf_probe_read_kernel(&family, sizeof(family), &sk->__sk_common.skc_family);
  if (family != AF_INET && family != AF_INET6)
    return 0;

  // Get destination info
  __u32 dst_ip = 0;
  __u16 dst_port = 0;
  bpf_probe_read_kernel(&dst_ip, sizeof(dst_ip), &sk->__sk_common.skc_daddr);
  bpf_probe_read_kernel(&dst_port, sizeof(dst_port),
                        &sk->__sk_common.skc_dport);

  // Prepare event
  struct http_event *event;
  event = bpf_ringbuf_reserve(&rb, sizeof(*event), 0);
  if (!event)
    return 0;

  event->event_type = EVENT_HTTP_REQUEST;
  event->dst_ip = dst_ip;
  event->dst_port = bpf_ntohs(dst_port);
  event->pid = bpf_get_current_pid_tgid() >> 32;
  event->timestamp = bpf_ktime_get_ns();
  event->data_len = size < MAX_HTTP_DATA ? size : MAX_HTTP_DATA;

  // Try to read the data from iov
  struct iov_iter *iter;
  bpf_probe_read_kernel(&iter, sizeof(iter), &msg->msg_iter);

  if (iter) {
    struct iovec *iov;
    bpf_probe_read_kernel(&iov, sizeof(iov), &iter->iov);
    if (iov) {
      void *base;
      bpf_probe_read_kernel(&base, sizeof(base), &iov->iov_base);
      if (base) {
        bpf_probe_read_user(event->data, event->data_len, base);
      }
    }
  }

  // Check if it's HTTP
  __u32 first_word = *(__u32 *)event->data;
  if (!is_http_request(first_word)) {
    bpf_ringbuf_discard(event, 0);
    return 0;
  }

  bpf_ringbuf_submit(event, 0);
  return 0;
}

// Track TCP connections for response correlation
struct conn_info {
  __u32 src_ip;
  __u32 dst_ip;
  __u16 src_port;
  __u16 dst_port;
  __u32 pid;
  __u64 request_ts;
};

struct {
  __uint(type, BPF_MAP_TYPE_HASH);
  __uint(max_entries, 10240);
  __type(key, __u64); // socket pointer
  __type(value, struct conn_info);
} conn_map SEC(".maps");

SEC("kprobe/tcp_recvmsg")
int trace_tcp_recvmsg(struct pt_regs *ctx) {
  __u64 cgroup_id = bpf_get_current_cgroup_id();

  __u32 *tracked = bpf_map_lookup_elem(&amp_containers, &cgroup_id);
  if (!tracked || *tracked != 1)
    return 0;

  struct sock *sk = (struct sock *)PT_REGS_PARM1(ctx);

  // Get socket key
  __u64 sk_key = (__u64)sk;

  // Look up connection info
  struct conn_info *conn = bpf_map_lookup_elem(&conn_map, &sk_key);

  // Prepare event for response
  struct http_event *event;
  event = bpf_ringbuf_reserve(&rb, sizeof(*event), 0);
  if (!event)
    return 0;

  event->event_type = EVENT_HTTP_RESPONSE;
  event->pid = bpf_get_current_pid_tgid() >> 32;
  event->timestamp = bpf_ktime_get_ns();

  if (conn) {
    event->src_ip = conn->dst_ip; // Response comes from dst
    event->dst_ip = conn->src_ip;
    event->src_port = conn->dst_port;
    event->dst_port = conn->src_port;
  }

  bpf_ringbuf_submit(event, 0);
  return 0;
}

char LICENSE[] SEC("license") = "GPL";
