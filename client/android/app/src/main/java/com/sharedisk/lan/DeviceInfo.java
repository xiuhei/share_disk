package com.sharedisk.lan;

import org.json.JSONObject;

final class DeviceInfo {
    final String id;
    final String name;
    final String platform;
    final String lastSeenAt;

    DeviceInfo(JSONObject json) {
        id = json.optString("id");
        name = json.optString("name", "未命名设备");
        platform = json.optString("platform");
        lastSeenAt = json.optString("last_seen_at");
    }
}
