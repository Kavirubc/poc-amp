// Minimal vmlinux.h for eBPF HTTP capture
// In production, generate with: bpftool btf dump file /sys/kernel/btf/vmlinux
// format c

#ifndef __VMLINUX_H__
#define __VMLINUX_H__

typedef unsigned char __u8;
typedef unsigned short __u16;
typedef unsigned int __u32;
typedef unsigned long long __u64;
typedef signed char __s8;
typedef signed short __s16;
typedef signed int __s32;
typedef signed long long __s64;

typedef unsigned long size_t;

typedef __u16 __be16;
typedef __u32 __be32;
typedef __u32 __wsum;

#define AF_INET 2
#define AF_INET6 10

#define BPF_MAP_TYPE_HASH 1
#define BPF_MAP_TYPE_PERF_EVENT_ARRAY 4
#define BPF_MAP_TYPE_RINGBUF 27

// Ethernet header
struct ethhdr {
  unsigned char h_dest[6];
  unsigned char h_source[6];
  __be16 h_proto;
};

// IP header
struct iphdr {
  __u8 ihl : 4, version : 4;
  __u8 tos;
  __be16 tot_len;
  __be16 id;
  __be16 frag_off;
  __u8 ttl;
  __u8 protocol;
  __u16 check;
  __be32 saddr;
  __be32 daddr;
};

// TCP header
struct tcphdr {
  __be16 source;
  __be16 dest;
  __be32 seq;
  __be32 ack_seq;
  __u16 res1 : 4, doff : 4, fin : 1, syn : 1, rst : 1, psh : 1, ack : 1,
      urg : 1, ece : 1, cwr : 1;
  __be16 window;
  __u16 check;
  __be16 urg_ptr;
};

// Socket structures
struct sock_common {
  union {
    struct {
      __be32 skc_daddr;
      __be32 skc_rcv_saddr;
    };
  };
  union {
    struct {
      __be16 skc_dport;
      __u16 skc_num;
    };
  };
  unsigned short skc_family;
  volatile unsigned char skc_state;
};

struct sock {
  struct sock_common __sk_common;
};

// Message structures
struct iovec {
  void *iov_base;
  __u64 iov_len;
};

struct iov_iter {
  unsigned int type;
  size_t iov_offset;
  size_t count;
  const struct iovec *iov;
  unsigned long nr_segs;
};

struct msghdr {
  void *msg_name;
  int msg_namelen;
  struct iov_iter msg_iter;
  void *msg_control;
  unsigned int msg_controllen;
  unsigned int msg_flags;
};

// x86_64 pt_regs with BOTH short and long register names.
// The system bpf_tracing.h uses short names (di, si, dx) for PT_REGS_PARM*
// macros, while some contexts use full names (rdi, rsi, rdx).
// Using anonymous unions makes both accessible.
struct pt_regs {
  unsigned long r15;
  unsigned long r14;
  unsigned long r13;
  unsigned long r12;
  union {
    unsigned long bp;
    unsigned long rbp;
  };
  union {
    unsigned long bx;
    unsigned long rbx;
  };
  unsigned long r11;
  unsigned long r10;
  unsigned long r9;
  unsigned long r8;
  union {
    unsigned long ax;
    unsigned long rax;
  };
  union {
    unsigned long cx;
    unsigned long rcx;
  };
  union {
    unsigned long dx;
    unsigned long rdx;
  };
  union {
    unsigned long si;
    unsigned long rsi;
  };
  union {
    unsigned long di;
    unsigned long rdi;
  };
  unsigned long orig_rax;
  union {
    unsigned long ip;
    unsigned long rip;
  };
  unsigned long cs;
  union {
    unsigned long flags;
    unsigned long eflags;
  };
  union {
    unsigned long sp;
    unsigned long rsp;
  };
  unsigned long ss;
};

// SK_BUFF for TC programs
struct __sk_buff {
  __u32 len;
  __u32 pkt_type;
  __u32 mark;
  __u32 queue_mapping;
  __u32 protocol;
  __u32 vlan_present;
  __u32 vlan_tci;
  __u32 vlan_proto;
  __u32 priority;
  __u32 ingress_ifindex;
  __u32 ifindex;
  __u32 tc_index;
  __u32 cb[5];
  __u32 hash;
  __u32 tc_classid;
  __u32 data;
  __u32 data_end;
  __u32 napi_id;
  __u32 family;
  __u32 remote_ip4;
  __u32 local_ip4;
  __u32 remote_ip6[4];
  __u32 local_ip6[4];
  __u32 remote_port;
  __u32 local_port;
  __u32 data_meta;
};

// TC action return values
#define TC_ACT_OK 0
#define TC_ACT_SHOT 2
#define TC_ACT_STOLEN 4
#define TC_ACT_REDIRECT 7

#endif /* __VMLINUX_H__ */
