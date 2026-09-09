package com.sharedisk.lan;

import org.json.JSONObject;

import java.time.Instant;

final class DeviceInfo {
    final String id;
    final String name;
    final String platform;
    final String lastSeenAt;
    final String status;
    final String connectionMode;
    final String connectedPeer;

    DeviceInfo(JSONObject json) {
        id = json.optString("id");
        name = json.optString("name", "未命名设备");
        platform = json.optString("platform");
        lastSeenAt = json.optString("last_seen_at");
        status = json.optString("status", "active");
        connectionMode = json.optString("connection_mode", "server");
        connectedPeer = json.optString("connected_peer");
    }

    boolean isOnline() {
        if (!"active".equals(status) || lastSeenAt.isEmpty()) return false;
        try {
            return Instant.parse(lastSeenAt).isAfter(Instant.now().minusSeconds(120));
        } catch (RuntimeException ignored) {
            return false;
        }
    }
}
