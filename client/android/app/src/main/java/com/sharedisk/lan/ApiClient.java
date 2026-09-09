package com.sharedisk.lan;

import android.content.ContentResolver;
import android.content.Context;
import android.content.SharedPreferences;
import android.database.Cursor;
import android.net.Uri;
import android.provider.OpenableColumns;

import org.json.JSONArray;
import org.json.JSONObject;

import java.io.BufferedInputStream;
import java.io.BufferedOutputStream;
import java.io.ByteArrayOutputStream;
import java.io.File;
import java.io.FileInputStream;
import java.io.FileOutputStream;
import java.io.IOException;
import java.io.InputStream;
import java.io.OutputStream;
import java.net.HttpURLConnection;
import java.net.URL;
import java.net.URLEncoder;
import java.nio.charset.StandardCharsets;
import java.security.MessageDigest;
import java.util.ArrayList;
import java.util.Base64;
import java.util.List;
import java.util.Locale;
import java.util.UUID;

final class ApiClient {
    interface Progress {
        void update(String message);
    }

    private static final String PREFS = "share_disk";
    private static final int BUFFER_SIZE = 1024 * 1024;
    private final Context context;
    private final SharedPreferences prefs;
    private final SecureCredentials credentials;

    ApiClient(Context context) {
        this.context = context.getApplicationContext();
        prefs = context.getSharedPreferences(PREFS, Context.MODE_PRIVATE);
        credentials = new SecureCredentials(this.context, prefs);
    }

    void saveEndpoints(String controlUrl, String agentUrl, String account) {
        prefs.edit()
                .putString("control_url", cleanBase(controlUrl))
                .putString("agent_url", cleanBase(agentUrl))
                .putString("account", account.trim())
                .apply();
    }

    String saved(String key) {
        return prefs.getString(key, "");
    }

    void bootstrap(String controlUrl, String account, String password, String bootstrapToken) throws Exception {
        JSONObject body = new JSONObject()
                .put("bootstrap_token", bootstrapToken)
                .put("account", account)
                .put("password", password)
                .put("device_name", android.os.Build.MODEL)
                .put("platform", "android");
        storeAuth(postJSON(cleanBase(controlUrl) + "/v1/auth/bootstrap", body, null));
    }

    void login(String controlUrl, String account, String password) throws Exception {
        String deviceId = prefs.getString("device_id", "");
        if (deviceId.isEmpty()) {
			String peerId = prefs.getString("peer_id", "");
			if (peerId.isEmpty()) {
				peerId = UUID.randomUUID().toString();
				prefs.edit().putString("peer_id", peerId).apply();
			}
			JSONObject enroll = new JSONObject()
					.put("account", account)
					.put("password", password)
					.put("name", android.os.Build.MODEL)
					.put("platform", "android")
					.put("peer_id", peerId);
			storeAuth(postJSON(cleanBase(controlUrl) + "/v1/auth/enroll", enroll, null));
			return;
        }
        JSONObject body = new JSONObject()
                .put("account", account)
                .put("password", password)
                .put("device_id", deviceId);
        storeAuth(postJSON(cleanBase(controlUrl) + "/v1/auth/login", body, null));
    }

    private void refreshAccessToken() throws Exception {
        String refresh = credentials.get("refresh_token");
        if (refresh.isEmpty()) {
            throw new IOException("登录已失效，请重新登录");
        }
        JSONObject body = new JSONObject().put("refresh_token", refresh);
        storeAuth(postJSON(saved("control_url") + "/v1/auth/refresh", body, null));
    }

    private synchronized String accessToken() throws Exception {
        long expiresAt = prefs.getLong("access_expires_at", 0L);
        if (System.currentTimeMillis() + 30_000L >= expiresAt) {
            refreshAccessToken();
        }
        String token = credentials.get("access_token");
        if (token.isEmpty()) {
            throw new IOException("请先登录");
        }
        return token;
    }

    private void storeAuth(JSONObject json) throws Exception {
        String access = json.getString("access_token");
        String refresh = json.getString("refresh_token");
        long expiresIn = json.optLong("expires_in", 900L);
        credentials.put("access_token", access);
        credentials.put("refresh_token", refresh);
        prefs.edit()
                .putString("user_id", json.getString("user_id"))
                .putString("device_id", json.getString("device_id"))
                .putLong("access_expires_at", System.currentTimeMillis() + expiresIn * 1000L)
                .apply();
    }

    boolean hasSession() {
        try {
            return !credentials.get("access_token").isEmpty();
        } catch (Exception ignored) {
            credentials.clearSession();
            return false;
        }
    }

    void logout() throws Exception {
        try {
            HttpURLConnection connection = open(saved("control_url") + "/v1/auth/logout", "POST", true);
            connection.setFixedLengthStreamingMode(0);
            connection.setDoOutput(true);
            connection.getOutputStream().close();
            int status = connection.getResponseCode();
            if (status != 204) throw responseError(connection);
            connection.disconnect();
        } finally {
            credentials.clearSession();
        }
    }

    List<LanFile> listFiles() throws Exception {
		return listFiles("", "", "name_asc");
	}

    private List<LanFile> listAgentFiles() throws Exception {
        JSONObject result = readJSON(open(saved("agent_url") + "/v1/lan/files", "GET", true));
        JSONArray files = result.getJSONArray("files");
        List<LanFile> output = new ArrayList<>();
        for (int i = 0; i < files.length(); i++) output.add(new LanFile(files.getJSONObject(i)));
        return output;
    }

	List<LanFile> listFiles(String folderId, String query, String sort) throws Exception {
		String address = saved("control_url") + "/v1/catalog/files?folder_id=" + encode(folderId) + "&q=" + encode(query) + "&sort=" + encode(sort);
		HttpURLConnection connection = open(address, "GET", true);
        JSONObject result = readJSON(connection);
        JSONArray files = result.getJSONArray("files");
        List<LanFile> output = new ArrayList<>();
        for (int i = 0; i < files.length(); i++) {
            output.add(new LanFile(files.getJSONObject(i)));
        }
        return output;
    }

	List<FolderInfo> listFolders() throws Exception {
		JSONArray values = readJSON(open(saved("control_url") + "/v1/catalog/folders", "GET", true)).getJSONArray("folders");
		List<FolderInfo> result = new ArrayList<>();
		for (int i = 0; i < values.length(); i++) result.add(new FolderInfo(values.getJSONObject(i)));
		return result;
	}

	FolderInfo createFolder(String parentId, String name) throws Exception {
		JSONObject body = new JSONObject().put("parent_id", parentId).put("name", name);
		return new FolderInfo(sendJSON(saved("control_url") + "/v1/catalog/folders", "POST", body));
	}

	void moveFile(LanFile file, FolderInfo folder) throws Exception {
		moveFile(file, folder.id);
	}

	void moveFile(LanFile file, String folderId) throws Exception {
		JSONObject body = new JSONObject().put("file_ids", new JSONArray().put(file.id)).put("folder_id", folderId);
		HttpURLConnection connection = open(saved("control_url") + "/v1/catalog/files/move", "POST", true);
		byte[] encoded = body.toString().getBytes(StandardCharsets.UTF_8);
		connection.setRequestProperty("Content-Type", "application/json");
		connection.setFixedLengthStreamingMode(encoded.length);
		connection.setDoOutput(true);
		try (OutputStream output = connection.getOutputStream()) { output.write(encoded); }
		int status = connection.getResponseCode();
		if (status != 204) throw responseError(connection);
		connection.disconnect();
	}

    List<LanFile> listTrash() throws Exception {
		HttpURLConnection connection = open(saved("control_url") + "/v1/catalog/trash", "GET", true);
        JSONObject result = readJSON(connection);
        JSONArray files = result.getJSONArray("files");
        List<LanFile> output = new ArrayList<>();
        for (int i = 0; i < files.length(); i++) output.add(new LanFile(files.getJSONObject(i)));
        return output;
    }

    List<DeviceInfo> listDevices() throws Exception {
        HttpURLConnection connection = open(saved("control_url") + "/v1/devices", "GET", true);
        JSONArray devices = readJSON(connection).getJSONArray("devices");
        List<DeviceInfo> output = new ArrayList<>();
        for (int i = 0; i < devices.length(); i++) output.add(new DeviceInfo(devices.getJSONObject(i)));
        return output;
    }

    void scheduleReplica(LanFile file, DeviceInfo target) throws Exception {
        JSONObject body = new JSONObject()
                .put("file_id", file.id)
                .put("target_device_id", target.id)
                .put("priority", 10);
        sendJSON(saved("control_url") + "/v1/transfers/", "POST", body);
    }

    List<TransferInfo> listTransfers() throws Exception {
        JSONArray values = readJSON(open(saved("control_url") + "/v1/transfers/?limit=100", "GET", true)).getJSONArray("transfers");
        List<TransferInfo> output = new ArrayList<>();
        for (int i = 0; i < values.length(); i++) output.add(new TransferInfo(values.getJSONObject(i)));
        return output;
    }

    void cancelTransfer(TransferInfo transfer) throws Exception {
        HttpURLConnection connection = open(saved("control_url") + "/v1/transfers/" + transfer.id + "/cancel", "POST", true);
        connection.setFixedLengthStreamingMode(0);
        connection.setDoOutput(true);
        connection.getOutputStream().close();
        int status = connection.getResponseCode();
        if (status != 204) throw responseError(connection);
        connection.disconnect();
    }

    String currentDeviceId() {
        return prefs.getString("device_id", "");
    }

    void deregisterDevice(DeviceInfo device) throws Exception {
        HttpURLConnection connection = open(saved("control_url") + "/v1/devices/" + device.id, "DELETE", true);
        int status = connection.getResponseCode();
        if (status != 204) throw responseError(connection);
        connection.disconnect();
    }

    String createShare(LanFile file) throws Exception {
        JSONObject body = new JSONObject()
                .put("file_id", file.id)
                .put("expires_in_seconds", 24 * 60 * 60)
                .put("max_downloads", 0);
        JSONObject share = sendJSON(saved("control_url") + "/v1/shares", "POST", body);
        return saved("control_url") + "/v1/public/shares/" + encode(share.getString("token"));
    }

    List<ShareInfo> listShares() throws Exception {
        JSONArray values = readJSON(open(saved("control_url") + "/v1/shares", "GET", true)).getJSONArray("shares");
        List<ShareInfo> result = new ArrayList<>();
        for (int i = 0; i < values.length(); i++) result.add(new ShareInfo(values.getJSONObject(i)));
        return result;
    }

    void revokeShare(ShareInfo share) throws Exception {
        HttpURLConnection connection = open(saved("control_url") + "/v1/shares/" + share.id, "DELETE", true);
        int status = connection.getResponseCode();
        if (status != 204) throw responseError(connection);
        connection.disconnect();
    }

    LanFile renameFile(LanFile file, String name) throws Exception {
		return applyAction(file, "rename", name);
    }

    LanFile trashFile(LanFile file) throws Exception {
		return applyAction(file, "trash", "");
    }

    LanFile restoreFile(LanFile file) throws Exception {
		return applyAction(file, "restore", "");
    }

    void purgeFile(LanFile file) throws Exception {
		applyAction(file, "purge", "");
    }

	void provisionAgent() throws Exception {
		HttpURLConnection connection = open(saved("agent_url") + "/v1/lan/coordinator/provision", "POST", true);
		connection.setFixedLengthStreamingMode(0);
		connection.setDoOutput(true);
		connection.getOutputStream().close();
		int status = connection.getResponseCode();
		if (status != 204) throw responseError(connection);
		connection.disconnect();
	}

    String coordinatorSummary() throws Exception {
        JSONObject value = readJSON(open(saved("agent_url") + "/v1/lan/coordinator/status", "GET", true));
        if (!value.optBoolean("configured")) return "Agent 未配置控制服务";
        if (!value.optBoolean("provisioned")) return "Agent 尚未登记";
        long pending = value.optLong("pending");
        long failed = value.optLong("failed");
        if (failed > 0) return "待登记 " + pending + "，需处理 " + failed;
        return "待登记 " + pending;
    }

	private LanFile applyAction(LanFile file, String action, String name) throws Exception {
		JSONArray ids = new JSONArray().put(file.id);
		JSONObject body = new JSONObject().put("operation_id", UUID.randomUUID().toString()).put("file_ids", ids).put("action", action);
		if (!name.isEmpty()) body.put("name", name);
		JSONObject response = sendJSON(saved("control_url") + "/v1/catalog/files/actions", "POST", body);
		return new LanFile(response.getJSONArray("files").getJSONObject(0));
	}

	private String agentBase(LanFile file) throws IOException {
		String value = file.replicaEndpoint.isEmpty() ? saved("agent_url") : file.replicaEndpoint;
		if (value.isEmpty()) throw new IOException("该文件没有可访问的在线副本");
		return cleanBase(value);
	}

	private String originAgentBase(LanFile file) throws IOException {
		String value = file.originEndpoint.isEmpty() ? saved("agent_url") : file.originEndpoint;
		if (value.isEmpty()) throw new IOException("文件来源设备当前没有可访问地址");
		return cleanBase(value);
	}

    private JSONObject sendJSON(String address, String method, JSONObject body) throws Exception {
        HttpURLConnection connection = open(address, method, true);
        if (body != null) {
            byte[] encoded = body.toString().getBytes(StandardCharsets.UTF_8);
            connection.setRequestProperty("Content-Type", "application/json");
            connection.setFixedLengthStreamingMode(encoded.length);
            connection.setDoOutput(true);
            try (OutputStream output = connection.getOutputStream()) { output.write(encoded); }
        } else if (method.equals("POST")) {
            connection.setFixedLengthStreamingMode(0);
            connection.setDoOutput(true);
            connection.getOutputStream().close();
        }
        return readJSON(connection);
    }

    LanFile upload(Uri source, Progress progress) throws Exception {
        ContentResolver resolver = context.getContentResolver();
        FileMeta meta = queryMeta(resolver, source);
        if (meta.size < 0) {
            throw new IOException("文件提供者没有返回大小，无法创建可恢复上传");
        }

        String resumeKey = "upload_url:" + source;
        String uploadUrl = prefs.getString(resumeKey, "");
        long offset = 0;
        if (!uploadUrl.isEmpty()) {
            try {
                offset = queryUploadOffset(uploadUrl);
            } catch (Exception ignored) {
                uploadUrl = "";
            }
        }
        if (uploadUrl.isEmpty()) {
            uploadUrl = createUpload(meta);
            prefs.edit().putString(resumeKey, uploadUrl).apply();
        }

        String fileId = "";
        String sha256 = "";
        byte[] buffer = new byte[BUFFER_SIZE];
        try (InputStream raw = resolver.openInputStream(source);
             BufferedInputStream input = new BufferedInputStream(requireInput(raw))) {
            skipFully(input, offset);
            while (offset < meta.size) {
                int wanted = (int) Math.min(buffer.length, meta.size - offset);
                int read = readUpTo(input, buffer, wanted);
                if (read <= 0) {
                    throw new IOException("文件在上传期间提前结束");
                }
                HttpURLConnection patch = open(uploadUrl, "PATCH", true);
                patch.setRequestProperty("Tus-Resumable", "1.0.0");
                patch.setRequestProperty("Upload-Offset", Long.toString(offset));
                patch.setRequestProperty("Content-Type", "application/offset+octet-stream");
                patch.setFixedLengthStreamingMode(read);
                patch.setDoOutput(true);
                try (OutputStream output = new BufferedOutputStream(patch.getOutputStream())) {
                    output.write(buffer, 0, read);
                }
                ensureSuccess(patch, 204);
                offset = Long.parseLong(patch.getHeaderField("Upload-Offset"));
                fileId = valueOrEmpty(patch.getHeaderField("X-Share-Disk-File-ID"));
                sha256 = valueOrEmpty(patch.getHeaderField("X-Share-Disk-SHA256"));
                progress.update(String.format(Locale.US, "上传 %.1f%%（%,d / %,d 字节）", offset * 100.0 / meta.size, offset, meta.size));
                patch.disconnect();
            }
        }
        prefs.edit().remove(resumeKey).apply();
        if (fileId.isEmpty() || sha256.isEmpty()) {
            List<LanFile> files = listAgentFiles();
            for (LanFile file : files) {
                if (file.name.equals(meta.name) && file.size == meta.size) {
                    return file;
                }
            }
            throw new IOException("上传完成但未取得文件发布结果");
        }
		LanFile local = new LanFile(new JSONObject()
                .put("id", fileId)
				.put("local_file_id", fileId)
                .put("object_id", sha256)
                .put("name", meta.name)
                .put("mime", meta.mime)
                .put("size", meta.size)
				.put("sha256", sha256)
                .put("status", "local_ready_pending_register"));
		for (int attempt = 0; attempt < 20; attempt++) {
			try {
				for (LanFile file : listFiles()) if (file.localFileId.equals(fileId)) return file;
			} catch (Exception controlUnavailable) {
				return local;
			}
			Thread.sleep(250L);
		}
		return local;
    }

    void download(LanFile file, Uri destination, Progress progress) throws Exception {
        File dir = new File(context.getCacheDir(), "downloads");
        if (!dir.isDirectory() && !dir.mkdirs()) {
            throw new IOException("无法创建下载缓存目录");
        }
        File part = new File(dir, file.id + ".part");
        long offset = part.isFile() ? part.length() : 0L;
        if (offset > file.size) {
            if (!part.delete()) throw new IOException("无法重置损坏的下载缓存");
            offset = 0L;
        }

        // A fully downloaded cache may survive process death after the network
        // step. Verify and publish it directly instead of issuing an invalid
        // Range request starting exactly at EOF.
        if (offset < file.size) {
			HttpURLConnection connection = open(agentBase(file) + "/v1/lan/files/" + file.contentLocalFileId + "/content", "GET", true);
            if (offset > 0) {
                connection.setRequestProperty("Range", "bytes=" + offset + "-");
                connection.setRequestProperty("If-Range", "\"" + file.sha256 + "\"");
            }
            int status = connection.getResponseCode();
            boolean append = offset > 0 && status == 206;
            if (status != 200 && status != 206) {
                throw responseError(connection);
            }
            String advertisedHash = valueOrEmpty(connection.getHeaderField("X-Content-SHA256"));
            if (!advertisedHash.isEmpty() && !advertisedHash.equalsIgnoreCase(file.sha256)) {
                connection.disconnect();
                throw new IOException("下载源返回了不同文件的 SHA-256");
            }
            if (append) {
                String contentRange = valueOrEmpty(connection.getHeaderField("Content-Range"));
                if (!contentRange.startsWith("bytes " + offset + "-")) {
                    connection.disconnect();
                    throw new IOException("下载源返回的续传偏移不一致");
                }
            } else {
                offset = 0L;
            }
            try (InputStream input = new BufferedInputStream(connection.getInputStream());
                 OutputStream output = new BufferedOutputStream(new FileOutputStream(part, append))) {
                byte[] buffer = new byte[BUFFER_SIZE];
                int read;
                long completed = offset;
                while ((read = input.read(buffer)) != -1) {
                    output.write(buffer, 0, read);
                    completed += read;
                    double percent = file.size == 0 ? 100.0 : completed * 100.0 / file.size;
                    progress.update(String.format(Locale.US, "下载 %.1f%%（%,d / %,d 字节）", percent, completed, file.size));
                }
            } finally {
                connection.disconnect();
            }
        }
        if (part.length() != file.size) {
            throw new IOException("下载大小不一致，可再次点击下载继续");
        }
        String actual = sha256(part);
        if (!actual.equalsIgnoreCase(file.sha256)) {
            if (!part.delete()) part.deleteOnExit();
            throw new IOException("SHA-256 校验失败，已丢弃缓存文件");
        }
        try (InputStream input = new BufferedInputStream(new FileInputStream(part));
             OutputStream output = new BufferedOutputStream(requireOutput(context.getContentResolver().openOutputStream(destination, "w")))) {
            byte[] buffer = new byte[BUFFER_SIZE];
            int read;
            while ((read = input.read(buffer)) != -1) output.write(buffer, 0, read);
        }
        if (!part.delete()) part.deleteOnExit();
    }

    private String createUpload(FileMeta meta) throws Exception {
        String endpoint = saved("agent_url") + "/v1/lan/uploads/";
        HttpURLConnection connection = open(endpoint, "POST", true);
        connection.setRequestProperty("Tus-Resumable", "1.0.0");
        connection.setRequestProperty("Upload-Length", Long.toString(meta.size));
        connection.setRequestProperty("Upload-Metadata",
                "filename " + Base64.getEncoder().encodeToString(meta.name.getBytes(StandardCharsets.UTF_8)) +
                        ",mime " + Base64.getEncoder().encodeToString(meta.mime.getBytes(StandardCharsets.UTF_8)));
        connection.setFixedLengthStreamingMode(0);
        connection.setDoOutput(true);
        connection.getOutputStream().close();
        ensureSuccess(connection, 201);
        String location = connection.getHeaderField("Location");
        connection.disconnect();
        if (location == null || location.isEmpty()) throw new IOException("上传服务未返回 Location");
        return new URL(new URL(endpoint), location).toString();
    }

    private long queryUploadOffset(String uploadUrl) throws Exception {
        HttpURLConnection connection = open(uploadUrl, "HEAD", true);
        connection.setRequestProperty("Tus-Resumable", "1.0.0");
        ensureSuccess(connection, 200);
        long offset = Long.parseLong(connection.getHeaderField("Upload-Offset"));
        connection.disconnect();
        return offset;
    }

    private HttpURLConnection open(String address, String method, boolean authenticated) throws Exception {
        if (address == null || address.isEmpty()) throw new IOException("请先配置服务地址");
        HttpURLConnection connection = (HttpURLConnection) new URL(address).openConnection();
        connection.setRequestMethod(method);
        connection.setConnectTimeout(10_000);
        connection.setReadTimeout(120_000);
        connection.setRequestProperty("Accept", "application/json");
        if (authenticated) connection.setRequestProperty("Authorization", "Bearer " + accessToken());
        return connection;
    }

    private JSONObject postJSON(String address, JSONObject body, String token) throws Exception {
        HttpURLConnection connection = (HttpURLConnection) new URL(address).openConnection();
        connection.setRequestMethod("POST");
        connection.setConnectTimeout(10_000);
        connection.setReadTimeout(30_000);
        connection.setRequestProperty("Content-Type", "application/json");
        if (token != null) connection.setRequestProperty("Authorization", "Bearer " + token);
        byte[] encoded = body.toString().getBytes(StandardCharsets.UTF_8);
        connection.setFixedLengthStreamingMode(encoded.length);
        connection.setDoOutput(true);
        try (OutputStream output = connection.getOutputStream()) {
            output.write(encoded);
        }
        return readJSON(connection);
    }

    private JSONObject readJSON(HttpURLConnection connection) throws Exception {
        int status = connection.getResponseCode();
        InputStream stream = status >= 200 && status < 300 ? connection.getInputStream() : connection.getErrorStream();
        String text = readText(stream);
        connection.disconnect();
        if (status < 200 || status >= 300) throw new HttpStatusException(status, errorMessage(text, status));
        return new JSONObject(text);
    }

    private static void ensureSuccess(HttpURLConnection connection, int expected) throws Exception {
        int status = connection.getResponseCode();
        if (status != expected) throw responseError(connection);
    }

    private static IOException responseError(HttpURLConnection connection) throws IOException {
        int status = connection.getResponseCode();
        return new HttpStatusException(status, errorMessage(readText(connection.getErrorStream()), status));
    }

    static boolean isRetryable(Exception error) {
        if (error instanceof HttpStatusException) {
            int status = ((HttpStatusException) error).status;
            return status == 408 || status == 425 || status == 429 || status >= 500;
        }
        return error instanceof IOException;
    }

    private static String errorMessage(String text, int status) {
        try {
            JSONObject json = new JSONObject(text);
            return "HTTP " + status + ": " + json.getJSONObject("error").optString("message", text);
        } catch (Exception ignored) {
            return "HTTP " + status + ": " + text.trim();
        }
    }

    private static String readText(InputStream input) throws IOException {
        if (input == null) return "";
        try (InputStream stream = input) {
            ByteArrayOutputStream output = new ByteArrayOutputStream();
            byte[] buffer = new byte[8192];
            int read;
            while ((read = stream.read(buffer)) != -1) output.write(buffer, 0, read);
            return output.toString(StandardCharsets.UTF_8.name());
        }
    }

    private static FileMeta queryMeta(ContentResolver resolver, Uri uri) throws IOException {
        String name = "upload.bin";
        long size = -1L;
        try (Cursor cursor = resolver.query(uri, new String[]{OpenableColumns.DISPLAY_NAME, OpenableColumns.SIZE}, null, null, null)) {
            if (cursor != null && cursor.moveToFirst()) {
                name = cursor.getString(0);
                if (!cursor.isNull(1)) size = cursor.getLong(1);
            }
        }
        String mime = resolver.getType(uri);
        if (mime == null || mime.isEmpty()) mime = "application/octet-stream";
        return new FileMeta(name, mime, size);
    }

    private static int readUpTo(InputStream input, byte[] buffer, int wanted) throws IOException {
        int total = 0;
        while (total < wanted) {
            int read = input.read(buffer, total, wanted - total);
            if (read == -1) break;
            total += read;
        }
        return total;
    }

    private static void skipFully(InputStream input, long count) throws IOException {
        long remaining = count;
        while (remaining > 0) {
            long skipped = input.skip(remaining);
            if (skipped > 0) {
                remaining -= skipped;
            } else if (input.read() == -1) {
                throw new IOException("无法恢复上传：源文件短于已提交偏移");
            } else {
                remaining--;
            }
        }
    }

    private static String sha256(File file) throws Exception {
        MessageDigest digest = MessageDigest.getInstance("SHA-256");
        try (InputStream input = new BufferedInputStream(new FileInputStream(file))) {
            byte[] buffer = new byte[BUFFER_SIZE];
            int read;
            while ((read = input.read(buffer)) != -1) digest.update(buffer, 0, read);
        }
        StringBuilder output = new StringBuilder(64);
        for (byte b : digest.digest()) output.append(String.format(Locale.US, "%02x", b & 0xff));
        return output.toString();
    }

    private static InputStream requireInput(InputStream input) throws IOException {
        if (input == null) throw new IOException("无法打开源文件");
        return input;
    }

    private static OutputStream requireOutput(OutputStream output) throws IOException {
        if (output == null) throw new IOException("无法打开目标文件");
        return output;
    }

    private static String cleanBase(String value) {
        String result = value == null ? "" : value.trim();
        while (result.endsWith("/")) result = result.substring(0, result.length() - 1);
        return result;
    }

    private static String valueOrEmpty(String value) {
        return value == null ? "" : value;
    }

	private static String encode(String value) throws Exception {
		return URLEncoder.encode(value == null ? "" : value, "UTF-8");
	}

    private static final class FileMeta {
        final String name;
        final String mime;
        final long size;

        FileMeta(String name, String mime, long size) {
            this.name = name;
            this.mime = mime;
            this.size = size;
        }
    }

    private static final class HttpStatusException extends IOException {
        final int status;

        HttpStatusException(int status, String message) {
            super(message);
            this.status = status;
        }
    }
}
