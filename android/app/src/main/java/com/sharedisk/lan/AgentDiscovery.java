package com.sharedisk.lan;

import android.content.Context;
import android.net.nsd.NsdManager;
import android.net.nsd.NsdServiceInfo;

import java.net.InetAddress;
import java.util.ArrayDeque;
import java.util.HashSet;
import java.util.Map;
import java.util.Set;

final class AgentDiscovery {
    interface Listener {
        void onAgentFound(String name, String url);
        void onDiscoveryStatus(String message);
    }

    private static final String SERVICE_TYPE = "_sharedisk._tcp.";
    private final NsdManager manager;
    private final Listener listener;
    private NsdManager.DiscoveryListener discoveryListener;
    private boolean resolving;
    private final ArrayDeque<NsdServiceInfo> pending = new ArrayDeque<>();
    private final Set<String> seen = new HashSet<>();

    AgentDiscovery(Context context, Listener listener) {
        manager = (NsdManager) context.getSystemService(Context.NSD_SERVICE);
        this.listener = listener;
    }

    void start() {
        stop();
        discoveryListener = new NsdManager.DiscoveryListener() {
            @Override public void onDiscoveryStarted(String serviceType) {
                listener.onDiscoveryStatus("正在局域网内查找 Ubuntu Agent…");
            }

            @Override public void onServiceFound(NsdServiceInfo service) {
                if (!service.getServiceType().startsWith("_sharedisk._tcp")) return;
                String key = service.getServiceName() + "|" + service.getServiceType();
                if (seen.add(key)) pending.add(service);
                resolveNext();
            }

            @Override public void onServiceLost(NsdServiceInfo service) { }

            @Override public void onDiscoveryStopped(String serviceType) { }

            @Override public void onStartDiscoveryFailed(String serviceType, int errorCode) {
                listener.onDiscoveryStatus("自动发现不可用（" + errorCode + "），可继续手工填写 Agent 地址");
                stop();
            }

            @Override public void onStopDiscoveryFailed(String serviceType, int errorCode) {
                discoveryListener = null;
            }
        };
        manager.discoverServices(SERVICE_TYPE, NsdManager.PROTOCOL_DNS_SD, discoveryListener);
    }

    void stop() {
        if (discoveryListener == null) return;
        try {
            manager.stopServiceDiscovery(discoveryListener);
        } catch (IllegalArgumentException ignored) {
            // Android may already have stopped a failed discovery session.
        }
        discoveryListener = null;
        resolving = false;
        pending.clear();
        seen.clear();
    }

    private void resolveNext() {
        if (resolving || pending.isEmpty()) return;
        resolving = true;
        manager.resolveService(pending.removeFirst(), resolveListener());
    }

    private NsdManager.ResolveListener resolveListener() {
        return new NsdManager.ResolveListener() {
            @Override public void onResolveFailed(NsdServiceInfo serviceInfo, int errorCode) {
                resolving = false;
                listener.onDiscoveryStatus("发现了 Agent，但地址解析失败（" + errorCode + "）");
                resolveNext();
            }

            @Override public void onServiceResolved(NsdServiceInfo serviceInfo) {
                resolving = false;
                Map<String, byte[]> attributes = serviceInfo.getAttributes();
                String api = text(attributes.get("api"));
                if (!api.isEmpty() && !api.equals("lan-v1")) {
                    resolveNext();
                    return;
                }
                String scheme = text(attributes.get("scheme"));
                if (!scheme.equals("https")) scheme = "http";
                InetAddress address = serviceInfo.getHost();
                if (address == null) {
                    resolveNext();
                    return;
                }
                String host = address.getHostAddress();
                if (host == null || host.isEmpty()) {
                    resolveNext();
                    return;
                }
                if (host.contains(":")) host = "[" + host.replace("%", "%25") + "]";
                listener.onAgentFound(serviceInfo.getServiceName(), scheme + "://" + host + ":" + serviceInfo.getPort());
                resolveNext();
            }
        };
    }

    private static String text(byte[] value) {
        return value == null ? "" : new String(value, java.nio.charset.StandardCharsets.UTF_8);
    }
}
