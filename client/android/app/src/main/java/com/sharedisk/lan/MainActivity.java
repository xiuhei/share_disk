package com.sharedisk.lan;

import android.Manifest;
import android.app.Activity;
import android.app.AlertDialog;
import android.app.Dialog;
import android.content.ClipData;
import android.content.ClipboardManager;
import android.content.Context;
import android.content.Intent;
import android.content.pm.PackageManager;
import android.content.res.ColorStateList;
import android.graphics.Typeface;
import android.graphics.drawable.GradientDrawable;
import android.net.Uri;
import android.os.Bundle;
import android.os.Build;
import android.os.Handler;
import android.os.Looper;
import android.text.Editable;
import android.text.TextWatcher;
import android.view.View;
import android.view.inputmethod.EditorInfo;
import android.view.inputmethod.InputMethodManager;
import android.widget.AdapterView;
import android.widget.Button;
import android.widget.ArrayAdapter;
import android.widget.EditText;
import android.widget.GridLayout;
import android.widget.ImageButton;
import android.widget.LinearLayout;
import android.widget.ProgressBar;
import android.widget.TextView;
import android.widget.Spinner;
import android.widget.Toast;

import androidx.work.WorkInfo;
import androidx.work.WorkManager;

import org.json.JSONObject;

import java.time.Instant;
import java.time.ZoneId;
import java.time.format.DateTimeFormatter;
import java.util.List;
import java.util.ArrayList;
import java.util.Collections;
import java.util.HashSet;
import java.util.LinkedHashMap;
import java.util.Locale;
import java.util.Map;
import java.util.Set;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;

public final class MainActivity extends Activity {
    private static final int PICK_UPLOAD = 1001;
    private static final int CREATE_DOWNLOAD = 1002;
    private static final int REQUEST_NOTIFICATIONS = 1003;
    private static final String STATE_SECTION = "section";
    private static final String STATE_SETTINGS = "settings";
    private static final String STATE_PREVIEW = "preview";
    private static final String STATE_FOLDER = "folder";
    private static final String STATE_QUERY = "query";
    private static final String STATE_SORT = "sort";
    private static final String STATE_FILTER = "filter";
    private static final String STATE_SEARCH_VISIBLE = "search_visible";

    private final ExecutorService executor = Executors.newSingleThreadExecutor();
    private final Handler uiHandler = new Handler(Looper.getMainLooper());
    private ApiClient api;
    private EditText controlUrl;
    private EditText agentUrl;
    private EditText account;
    private EditText password;
    private EditText bootstrapToken;
    private TextView status;
    private LinearLayout statusPanel;
    private ProgressBar statusProgress;
    private Button statusDismiss;
    private TextView screenTitle;
    private LinearLayout fileList;
    private LinearLayout trashList;
    private LinearLayout discoveryList;
    private LinearLayout transferList;
    private LinearLayout deviceList;
    private LinearLayout folderList;
    private LinearLayout shareList;
    private LinearLayout backgroundDownloadList;
    private EditText searchQuery;
    private View searchPanel;
    private Button clearSearchButton;
    private TextView fileSummary;
    private Spinner sortSpinner;
    private Button createFolderButton;
    private ImageButton viewModeButton;
    private Spinner filterSpinner;
    private Spinner deviceFilterSpinner;
    private View selectionBar;
    private TextView selectionCount;
    private String selectedFolderId = "";
    private Button bootstrapButton;
    private Button loginButton;
    private Button logoutButton;
    private ImageButton uploadButton;
    private Button refreshButton;
    private Button discoverButton;
    private ImageButton searchButton;
    private ImageButton settingsButton;
    private ImageButton globalRefreshButton;
    private Button previewButton;
    private ImageButton filesTab;
    private ImageButton transfersTab;
    private ImageButton devicesTab;
    private ImageButton trashTab;
    private LinearLayout connectionPanel;
    private LinearLayout filesSection;
    private LinearLayout transfersSection;
    private LinearLayout devicesSection;
    private LinearLayout trashSection;
    private LanFile pendingDownload;
    private AgentDiscovery discovery;
    private final Map<String, String> discoveredAgents = new LinkedHashMap<>();
    private boolean previewMode;
    private boolean busy;
    private boolean ignoreInitialSortEvent = true;
    private boolean ignoreInitialFilterEvent = true;
    private Runnable pendingSearch;
    private LinearLayout currentSection;
    private boolean settingsVisible;
    private boolean destroyed;
    private Runnable pendingNotificationAction;
    private final Runnable transferPoll = this::refreshBackgroundTransfersOnly;
    private final List<LanFile> lastFiles = new ArrayList<>();
    private final List<DeviceInfo> lastDevices = new ArrayList<>();
    private final List<String> deviceFilterIds = new ArrayList<>();
    private final Map<String, String> deviceAliases = new LinkedHashMap<>();
    private String activeTypeFilter = "all";
    private String activeDeviceFilter = "all";
    private boolean gridView;
    private final Set<String> selectedFileIds = new HashSet<>();

    @Override
    protected void onCreate(Bundle savedInstanceState) {
        super.onCreate(savedInstanceState);
        setContentView(R.layout.activity_main);
        api = new ApiClient(this);

        controlUrl = findViewById(R.id.controlUrl);
        agentUrl = findViewById(R.id.agentUrl);
        account = findViewById(R.id.account);
        password = findViewById(R.id.password);
        bootstrapToken = findViewById(R.id.bootstrapToken);
        status = findViewById(R.id.status);
        statusPanel = findViewById(R.id.statusPanel);
        statusProgress = findViewById(R.id.statusProgress);
        statusDismiss = findViewById(R.id.statusDismiss);
        screenTitle = findViewById(R.id.screenTitle);
        fileList = findViewById(R.id.fileList);
        trashList = findViewById(R.id.trashList);
        discoveryList = findViewById(R.id.discoveryList);
        transferList = findViewById(R.id.transferList);
        deviceList = findViewById(R.id.deviceList);
        folderList = findViewById(R.id.folderList);
        shareList = findViewById(R.id.shareList);
        backgroundDownloadList = findViewById(R.id.backgroundDownloadList);
        searchQuery = findViewById(R.id.searchQuery);
        searchPanel = findViewById(R.id.searchPanel);
        clearSearchButton = findViewById(R.id.clearSearchButton);
        fileSummary = findViewById(R.id.fileSummary);
        sortSpinner = findViewById(R.id.sortSpinner);
        createFolderButton = findViewById(R.id.createFolderButton);
        viewModeButton = findViewById(R.id.viewModeButton);
        filterSpinner = findViewById(R.id.filterSpinner);
        deviceFilterSpinner = findViewById(R.id.deviceFilterSpinner);
        filterSpinner.setAdapter(compactSpinnerAdapter(
                new String[]{"全部", "近 7 天", "图片", "文档", "视频"}));
        deviceFilterIds.add("all");
        deviceFilterSpinner.setAdapter(compactSpinnerAdapter(new String[]{"全部设备"}));
        selectionBar = findViewById(R.id.selectionBar);
        selectionCount = findViewById(R.id.selectionCount);
        sortSpinner.setAdapter(compactSpinnerAdapter(
                new String[]{"名称 ↑", "名称 ↓", "最新", "最早", "大小 ↓", "大小 ↑"}));
        bootstrapButton = findViewById(R.id.bootstrapButton);
        loginButton = findViewById(R.id.loginButton);
        logoutButton = findViewById(R.id.logoutButton);
        uploadButton = findViewById(R.id.uploadButton);
        refreshButton = findViewById(R.id.refreshButton);
        discoverButton = findViewById(R.id.discoverButton);
        searchButton = findViewById(R.id.searchButton);
        settingsButton = findViewById(R.id.settingsButton);
        globalRefreshButton = findViewById(R.id.globalRefreshButton);
        previewButton = findViewById(R.id.previewButton);
        filesTab = findViewById(R.id.filesTab);
        transfersTab = findViewById(R.id.transfersTab);
        devicesTab = findViewById(R.id.devicesTab);
        trashTab = findViewById(R.id.trashTab);
        connectionPanel = findViewById(R.id.connectionPanel);
        filesSection = findViewById(R.id.filesSection);
        transfersSection = findViewById(R.id.transfersSection);
        devicesSection = findViewById(R.id.devicesSection);
        trashSection = findViewById(R.id.trashSection);
        gridView = getSharedPreferences("share_disk", MODE_PRIVATE).getBoolean("grid_view", false);
        updateViewModeButton();
        selectFilter("all", false);
        discovery = new AgentDiscovery(this, new AgentDiscovery.Listener() {
            @Override public void onAgentFound(String name, String url) {
                runOnUiThread(() -> addDiscoveredAgent(name, url));
            }

            @Override public void onDiscoveryStatus(String message) {
                runOnUiThread(() -> setStatus(message));
            }
        });

        controlUrl.setText(api.saved("control_url"));
        agentUrl.setText(api.saved("agent_url"));
        account.setText(api.saved("account"));

        bootstrapButton.setOnClickListener(v -> authenticate(true));
        loginButton.setOnClickListener(v -> authenticate(false));
        logoutButton.setOnClickListener(v -> runTask("正在注销…", () -> {
            api.logout();
            runOnUiThread(this::showSettings);
            return "已注销当前会话";
        }, false));
        uploadButton.setOnClickListener(v -> {
            if (previewMode) {
                new AlertDialog.Builder(this)
                        .setTitle("连接后即可上传")
                        .setMessage("当前展示的是示例数据。连接真实设备后，将使用 Android 系统文件选择器上传本机文件。")
                        .setNegativeButton("继续预览", null)
                        .setPositiveButton("去连接", (dialog, which) -> showSettings())
                        .show();
            } else {
                showAddActions();
            }
        });
        refreshButton.setOnClickListener(v -> refreshWithFeedback());
        createFolderButton.setOnClickListener(v -> {
            if (previewMode) setStatus("示例预览 · 连接真实设备后可新建文件夹"); else promptCreateFolder();
        });
        discoverButton.setOnClickListener(v -> {
            discoveredAgents.clear();
            discoveryList.removeAllViews();
            discovery.start();
        });
        searchButton.setOnClickListener(v -> toggleSearch());
        settingsButton.setOnClickListener(v -> {
            if (settingsVisible) returnToCurrentSection(); else showSettings();
        });
        globalRefreshButton.setOnClickListener(v -> refreshWithFeedback());
        previewButton.setOnClickListener(v -> showPreview());
        filesTab.setOnClickListener(v -> showSection(filesSection, filesTab));
        transfersTab.setOnClickListener(v -> showSection(transfersSection, transfersTab));
        devicesTab.setOnClickListener(v -> showSection(devicesSection, devicesTab));
        trashTab.setOnClickListener(v -> showSection(trashSection, trashTab));
        viewModeButton.setOnClickListener(v -> {
            gridView = !gridView;
            getSharedPreferences("share_disk", MODE_PRIVATE).edit().putBoolean("grid_view", gridView).apply();
            updateViewModeButton();
            renderFiles(lastFiles);
        });
        filterSpinner.setOnItemSelectedListener(new AdapterView.OnItemSelectedListener() {
            @Override public void onItemSelected(AdapterView<?> parent, View view, int position, long id) {
                String[] filters = {"all", "recent", "images", "documents", "videos"};
                activeTypeFilter = filters[position];
                if (ignoreInitialFilterEvent) {
                    ignoreInitialFilterEvent = false;
                    return;
                }
                renderFiles(lastFiles);
            }
            @Override public void onNothingSelected(AdapterView<?> parent) {}
        });
        deviceFilterSpinner.setOnItemSelectedListener(new AdapterView.OnItemSelectedListener() {
            @Override public void onItemSelected(AdapterView<?> parent, View view, int position, long id) {
                if (position < 0 || position >= deviceFilterIds.size()) return;
                activeDeviceFilter = deviceFilterIds.get(position);
                renderFiles(lastFiles);
            }
            @Override public void onNothingSelected(AdapterView<?> parent) {}
        });
        findViewById(R.id.selectionClose).setOnClickListener(v -> clearSelection());
        findViewById(R.id.selectionDownload).setOnClickListener(v -> downloadSelection());
        findViewById(R.id.selectionShare).setOnClickListener(v -> shareSelection());
        findViewById(R.id.selectionMove).setOnClickListener(v -> moveSelection());
        findViewById(R.id.selectionTrash).setOnClickListener(v -> trashSelection());
        findViewById(R.id.profileConnectionButton).setOnClickListener(v -> showSettings());
        findViewById(R.id.profileStorageButton).setOnClickListener(v -> setStatus("存储偏好将跟随所连接的存储设备策略"));
        findViewById(R.id.profileTransferButton).setOnClickListener(v -> showSection(transfersSection, transfersTab));
        findViewById(R.id.profileAppearanceButton).setOnClickListener(v -> setStatus("当前使用跟随系统的明亮外观"));
        findViewById(R.id.profileSecurityButton).setOnClickListener(v -> setStatus("凭据已使用 Android 加密存储保护"));
        findViewById(R.id.profileAboutButton).setOnClickListener(v -> new AlertDialog.Builder(this)
                .setTitle("Share Disk")
                .setMessage("私人分布式云盘 · Android 0.3.0\n数据优先保存在你自己的设备上。")
                .setPositiveButton("关闭", null)
                .show());
        statusDismiss.setOnClickListener(v -> statusPanel.setVisibility(View.GONE));
        clearSearchButton.setOnClickListener(v -> {
            searchQuery.setText("");
            searchQuery.clearFocus();
        });
        searchQuery.setOnEditorActionListener((view, actionId, event) -> {
            if (actionId == EditorInfo.IME_ACTION_SEARCH) {
                view.clearFocus();
                ((InputMethodManager) getSystemService(Context.INPUT_METHOD_SERVICE)).hideSoftInputFromWindow(view.getWindowToken(), 0);
                refreshFiles();
                return true;
            }
            return false;
        });
        searchQuery.addTextChangedListener(new TextWatcher() {
            @Override public void beforeTextChanged(CharSequence value, int start, int count, int after) {}
            @Override public void onTextChanged(CharSequence value, int start, int before, int count) {
                clearSearchButton.setVisibility(value.length() == 0 ? View.GONE : View.VISIBLE);
                if (pendingSearch != null) uiHandler.removeCallbacks(pendingSearch);
                pendingSearch = () -> {
                    if (!settingsVisible && !busy) refreshFiles();
                };
                uiHandler.postDelayed(pendingSearch, 450);
            }
            @Override public void afterTextChanged(Editable value) {}
        });
        sortSpinner.setOnItemSelectedListener(new AdapterView.OnItemSelectedListener() {
            @Override public void onItemSelected(AdapterView<?> parent, View view, int position, long id) {
                if (ignoreInitialSortEvent) {
                    ignoreInitialSortEvent = false;
                    return;
                }
                if (!settingsVisible && !busy) refreshFiles();
            }
            @Override public void onNothingSelected(AdapterView<?> parent) {}
        });
        if (api.hasSession()) {
            connectionPanel.setVisibility(View.GONE);
            if (savedInstanceState != null) restoreUiState(savedInstanceState);
            else showSection(filesSection, filesTab);
            setStatus("已有登录会话，正在读取设备文件…");
            if (!settingsVisible) refreshFiles();
        } else {
            showPreview();
            if (savedInstanceState != null && savedInstanceState.getBoolean(STATE_SETTINGS, false)) restoreUiState(savedInstanceState);
        }
        handleLaunchIntent(getIntent());
    }

    @Override
    protected void onNewIntent(Intent intent) {
        super.onNewIntent(intent);
        setIntent(intent);
        handleLaunchIntent(intent);
    }

    private void handleLaunchIntent(Intent intent) {
        if (intent != null && "transfers".equals(intent.getStringExtra("open_section"))) {
            showSection(transfersSection, transfersTab);
            intent.removeExtra("open_section");
        }
    }

    private void authenticate(boolean bootstrap) {
        previewMode = false;
        saveEndpoints();
        String accountValue = account.getText().toString().trim();
        String passwordValue = password.getText().toString();
        String control = controlUrl.getText().toString().trim();
        String agent = agentUrl.getText().toString().trim();
        controlUrl.setError(null);
        agentUrl.setError(null);
        account.setError(null);
        password.setError(null);
        bootstrapToken.setError(null);
        if (control.isEmpty()) {
            controlUrl.setError("请输入控制服务地址");
            controlUrl.requestFocus();
            setStatus("请先填写控制服务地址");
            return;
        }
        if (agent.isEmpty()) {
            agentUrl.setError("请输入或自动发现存储 Agent");
            agentUrl.requestFocus();
            setStatus("请填写存储 Agent 地址，或使用自动发现");
            return;
        }
        if (accountValue.isEmpty()) {
            account.setError("请输入账号");
            account.requestFocus();
            setStatus("请输入账号");
            return;
        }
        if (passwordValue.isEmpty()) {
            password.setError("请输入密码");
            password.requestFocus();
            setStatus("请输入密码");
            return;
        }
        runTask(bootstrap ? "正在初始化账号…" : "正在登录…", () -> {
            if (bootstrap) {
                String token = bootstrapToken.getText().toString().trim();
                if (token.isEmpty()) {
                    runOnUiThread(() -> bootstrapToken.setError("请输入首次初始化令牌"));
                    throw new IllegalArgumentException("首次初始化令牌不能为空");
                }
                api.bootstrap(control, accountValue, passwordValue, token);
            } else {
                api.login(control, accountValue, passwordValue);
            }
            api.provisionAgent();
            runOnUiThread(() -> {
                password.setText("");
                bootstrapToken.setText("");
                connectionPanel.setVisibility(View.GONE);
            });
            return "登录成功，正在读取文件…";
        }, true);
    }

    private void chooseUpload() {
        chooseUpload("*/*");
    }

    private void chooseUpload(String mimeType) {
        runWithTransferNotificationPermission(() -> openUploadPicker(mimeType));
    }

    private void openUploadPicker(String mimeType) {
        saveEndpoints();
        Intent intent = new Intent(Intent.ACTION_OPEN_DOCUMENT);
        intent.addCategory(Intent.CATEGORY_OPENABLE);
        intent.setType(mimeType);
        intent.putExtra(Intent.EXTRA_ALLOW_MULTIPLE, true);
        startActivityForResult(intent, PICK_UPLOAD);
    }

    private void showAddActions() {
        Map<String, Runnable> actions = new LinkedHashMap<>();
        actions.put("上传文件", this::chooseUpload);
        actions.put("上传照片", () -> chooseUpload("image/*"));
        actions.put("创建文件夹", this::promptCreateFolder);
        showActionSheet("添加到 Share Disk", actions);
    }

    private void chooseDownload(LanFile file) {
        if (previewMode) {
            new AlertDialog.Builder(this)
                    .setTitle("下载文件")
                    .setMessage("示例模式不会创建真实文件。连接设备后，这里会打开 Android 系统保存位置选择器并在完成后校验 SHA-256。")
                    .setPositiveButton("知道了", null)
                    .show();
            return;
        }
        runWithTransferNotificationPermission(() -> openDownloadPicker(file));
    }

    private void openDownloadPicker(LanFile file) {
        pendingDownload = file;
        Intent intent = new Intent(Intent.ACTION_CREATE_DOCUMENT);
        intent.addCategory(Intent.CATEGORY_OPENABLE);
        intent.setType(file.mime);
        intent.putExtra(Intent.EXTRA_TITLE, file.name);
        startActivityForResult(intent, CREATE_DOWNLOAD);
    }

    private void runWithTransferNotificationPermission(Runnable action) {
        if (Build.VERSION.SDK_INT < Build.VERSION_CODES.TIRAMISU
                || checkSelfPermission(Manifest.permission.POST_NOTIFICATIONS) == PackageManager.PERMISSION_GRANTED) {
            action.run();
            return;
        }
        boolean requested = getSharedPreferences("share_disk", MODE_PRIVATE)
                .getBoolean("notification_permission_requested", false);
        if (requested) {
            setStatus("通知未开启，传输仍会执行；可在系统任务管理器查看并停止后台任务");
            action.run();
            return;
        }
        pendingNotificationAction = action;
        getSharedPreferences("share_disk", MODE_PRIVATE).edit()
                .putBoolean("notification_permission_requested", true).apply();
        requestPermissions(new String[]{Manifest.permission.POST_NOTIFICATIONS}, REQUEST_NOTIFICATIONS);
    }

    @Override
    public void onRequestPermissionsResult(int requestCode, String[] permissions, int[] grantResults) {
        super.onRequestPermissionsResult(requestCode, permissions, grantResults);
        if (requestCode != REQUEST_NOTIFICATIONS) return;
        Runnable action = pendingNotificationAction;
        pendingNotificationAction = null;
        if (grantResults.length == 0 || grantResults[0] != PackageManager.PERMISSION_GRANTED) {
            setStatus("未开启传输通知，任务仍可运行；可稍后在系统设置中开启");
        }
        if (action != null) action.run();
    }

    @Override
    protected void onActivityResult(int requestCode, int resultCode, Intent data) {
        super.onActivityResult(requestCode, resultCode, data);
        if (resultCode != RESULT_OK || data == null) {
            if (requestCode == CREATE_DOWNLOAD) pendingDownload = null;
            setStatus("已取消选择，没有更改任何文件");
            return;
        }
        if (requestCode == PICK_UPLOAD) {
            List<Uri> selected = new ArrayList<>();
            if (data.getClipData() != null) {
                for (int index = 0; index < data.getClipData().getItemCount(); index++) {
                    selected.add(data.getClipData().getItemAt(index).getUri());
                }
            } else if (data.getData() != null) {
                selected.add(data.getData());
            }
            if (selected.isEmpty()) {
                setStatus("没有选择可上传的文件");
                return;
            }
            for (Uri uri : selected) {
                try {
                    getContentResolver().takePersistableUriPermission(uri, Intent.FLAG_GRANT_READ_URI_PERMISSION);
                } catch (SecurityException ignored) {
                    // Some document providers grant access only for the current process.
                }
                UploadWorker.enqueue(this, uri);
            }
            setStatus("已将 " + selected.size() + " 个文件加入后台上传，可在“传输”中查看进度");
            refreshFiles();
        } else if (requestCode == CREATE_DOWNLOAD && pendingDownload != null) {
            Uri uri = data.getData();
            if (uri == null) {
                pendingDownload = null;
                setStatus("未选择保存位置，下载已取消");
                return;
            }
            LanFile file = pendingDownload;
            pendingDownload = null;
            try {
                getContentResolver().takePersistableUriPermission(uri, Intent.FLAG_GRANT_WRITE_URI_PERMISSION);
            } catch (SecurityException ignored) {
                // Worker may still use the provider's current grant until process death.
            }
            java.util.UUID workID = DownloadWorker.enqueue(this, file, uri);
            setStatus("后台下载已入队：" + file.name + "\n任务 " + workID);
            refreshFiles();
        }
    }

    private void refreshFiles() {
        if (previewMode) {
            showPreview();
            return;
        }
        saveEndpoints();
        String folderFilter = selectedFolderId;
        String queryFilter = searchQuery.getText().toString();
        String sortOrder = selectedSort();
        runTask("正在刷新文件列表…", () -> {
            List<String> warnings = new ArrayList<>();
            optionalLoad("Agent 登记", () -> {
                api.provisionAgent();
                return true;
            }, false, warnings);
            List<LanFile> files = api.listFiles(folderFilter, queryFilter, sortOrder);
            List<LanFile> trash = optionalLoad("回收站", api::listTrash, Collections.emptyList(), warnings);
            List<TransferInfo> transfers = optionalLoad("跨设备任务", api::listTransfers, Collections.emptyList(), warnings);
            List<DeviceInfo> devices = optionalLoad("设备状态", api::listDevices, Collections.emptyList(), warnings);
            List<FolderInfo> folders = optionalLoad("文件夹", api::listFolders, Collections.emptyList(), warnings);
            List<ShareInfo> shares = optionalLoad("分享记录", api::listShares, Collections.emptyList(), warnings);
            String coordinator = optionalLoad("Agent 状态", api::coordinatorSummary, "Agent 状态未知", warnings);
            List<WorkInfo> backgroundTransfers = new ArrayList<>();
            backgroundTransfers.addAll(optionalLoad("上传任务", () -> WorkManager.getInstance(this).getWorkInfosByTag(UploadWorker.TAG).get(), Collections.emptyList(), warnings));
            backgroundTransfers.addAll(optionalLoad("下载任务", () -> WorkManager.getInstance(this).getWorkInfosByTag(DownloadWorker.TAG).get(), Collections.emptyList(), warnings));
            runOnUiThread(() -> {
                renderFiles(files);
                renderTrash(trash);
                renderTransfers(transfers);
                renderDevices(devices);
                renderFolders(folders);
                renderShares(shares);
                renderBackgroundDownloads(backgroundTransfers);
            });
            if (!warnings.isEmpty()) {
                return "已显示可用数据 · " + String.join("、", warnings) + " 暂不可用，点击刷新可重试";
            }
            return "已连接：" + files.size() + " 个文件，回收站 " + trash.size() + " 项，" + coordinator;
        }, false);
    }

    private void refreshWithFeedback() {
        if (busy) return;
        globalRefreshButton.animate().cancel();
        globalRefreshButton.setRotation(0f);
        globalRefreshButton.animate().rotation(360f).setDuration(550L).start();
        if (previewMode) {
            setBusy(true);
            setStatus("正在刷新文件列表…");
            uiHandler.postDelayed(() -> {
                if (destroyed) return;
                showPreview();
                setBusy(false);
                setStatus("文件列表已刷新");
                uiHandler.postDelayed(() -> {
                    if (!destroyed && !busy) statusPanel.setVisibility(View.GONE);
                }, 1400L);
            }, 550L);
            return;
        }
        setStatus("正在刷新文件列表…");
        refreshFiles();
    }

    private ArrayAdapter<String> compactSpinnerAdapter(String[] values) {
        ArrayAdapter<String> adapter = new ArrayAdapter<>(this, R.layout.spinner_item, values);
        adapter.setDropDownViewResource(R.layout.spinner_dropdown_item);
        return adapter;
    }

    private interface Loader<T> {
        T load() throws Exception;
    }

    private <T> T optionalLoad(String label, Loader<T> loader, T fallback, List<String> warnings) {
        try {
            return loader.load();
        } catch (Exception ignored) {
            warnings.add(label);
            return fallback;
        }
    }

    private void renderShares(List<ShareInfo> shares) {
        shareList.removeAllViews();
        if (shares.isEmpty()) {
            addEmptyState(shareList, "还没有分享", "", null, null);
            return;
        }
        for (ShareInfo share : shares) {
            LinearLayout row = new LinearLayout(this);
            row.setOrientation(LinearLayout.VERTICAL);
            styleCard(row);
            TextView detail = new TextView(this);
            String limit = share.maxDownloads == 0 ? "不限次数" : share.downloadCount + "/" + share.maxDownloads + " 次";
            detail.setText(getString(R.string.share_row_format, share.fileName, share.status,
                    displayTime(share.expiresAt), limit));
            row.addView(detail);
            if (share.status.equals("active")) {
                Button revoke = new Button(this);
                revoke.setText("撤销分享");
                revoke.setOnClickListener(v -> runTask("正在撤销分享…", () -> {
                    api.revokeShare(share);
                    return "分享已撤销：" + share.fileName;
                }, true));
                row.addView(revoke);
            }
            shareList.addView(row);
        }
    }

    private void renderBackgroundDownloads(List<WorkInfo> work) {
        backgroundDownloadList.removeAllViews();
        if (work.isEmpty()) {
            addEmptyState(backgroundDownloadList, "暂无后台传输", "", null, null);
            return;
        }
        int start = Math.max(0, work.size() - 10);
        for (int i = start; i < work.size(); i++) {
            WorkInfo item = work.get(i);
            LinearLayout card = new LinearLayout(this);
            card.setOrientation(LinearLayout.VERTICAL);
            styleCard(card);
            String message = item.getProgress().getString("message");
            if (item.getState().isFinished()) message = item.getOutputData().getString("message");
            TextView detail = new TextView(this);
            String stateLabel = workStateLabel(item.getState());
            detail.setText(message == null || message.isEmpty()
                    ? stateLabel
                    : getString(R.string.work_row_with_message_format, stateLabel, message));
            card.addView(detail);
            if (!item.getState().isFinished()) {
                Button cancel = new Button(this);
                cancel.setAllCaps(false);
                cancel.setText("取消传输");
                styleDangerButton(cancel);
                cancel.setOnClickListener(v -> new AlertDialog.Builder(this)
                        .setTitle("取消后台传输？")
                        .setNegativeButton("继续传输", null)
                        .setPositiveButton("取消传输", (dialog, which) -> {
                            WorkManager.getInstance(this).cancelWorkById(item.getId());
                            setStatus("已请求取消后台传输");
                            refreshFiles();
                        }).show());
                card.addView(cancel);
            }
            backgroundDownloadList.addView(card);
        }
    }

    private String selectedSort() {
        switch (sortSpinner.getSelectedItemPosition()) {
        case 1: return "name_desc";
        case 2: return "newest";
        case 3: return "oldest";
        case 4: return "size_desc";
        case 5: return "size_asc";
        default: return "name_asc";
        }
    }

    private void renderFolders(List<FolderInfo> folders) {
        folderList.removeAllViews();
        Button all = new Button(this);
        all.setText(selectedFolderId.isEmpty() ? "✓ 全部文件" : "全部文件");
        all.setAllCaps(false);
        all.setOnClickListener(v -> { selectedFolderId = ""; refreshFiles(); });
        folderList.addView(all);
        for (FolderInfo folder : folders) {
            Button choice = new Button(this);
            choice.setText(folder.id.equals(selectedFolderId)
                    ? getString(R.string.selected_folder_format, folder.name)
                    : folder.name);
            choice.setAllCaps(false);
            choice.setOnClickListener(v -> { selectedFolderId = folder.id; refreshFiles(); });
            folderList.addView(choice);
        }
    }

    private void promptCreateFolder() {
        EditText input = new EditText(this);
        input.setHint("文件夹名称");
        new AlertDialog.Builder(this).setTitle("新建文件夹").setView(input).setNegativeButton("取消", null)
                .setPositiveButton("创建", (dialog, which) -> {
                    String name = input.getText().toString().trim();
                    if (name.isEmpty()) {
                        setStatus("文件夹名称不能为空");
                        return;
                    }
                    runTask("正在创建文件夹…", () -> {
                    FolderInfo folder = api.createFolder(selectedFolderId, name);
                    selectedFolderId = folder.id;
                    return "已创建 " + folder.name;
                }, true);
                }).show();
    }

    private void renderTransfers(List<TransferInfo> transfers) {
        transferList.removeAllViews();
        if (transfers.isEmpty()) {
            addEmptyState(transferList, "暂无跨设备任务", "", null, null);
            return;
        }
        for (TransferInfo transfer : transfers) {
            LinearLayout card = new LinearLayout(this);
            card.setOrientation(LinearLayout.VERTICAL);
            styleCard(card);
            TextView detail = new TextView(this);
            detail.setText(getString(R.string.transfer_row_format, transferStateLabel(transfer.state),
                    transfer.attempt, prefix(transfer.objectId, 20)));
            card.addView(detail);
            if (!transfer.terminal()) {
                Button cancel = new Button(this);
                cancel.setText("取消任务");
                styleDangerButton(cancel);
                cancel.setOnClickListener(v -> new AlertDialog.Builder(this)
                        .setTitle("取消传输？")
                        .setNegativeButton("继续传输", null)
                        .setPositiveButton("取消传输", (dialog, which) -> {
                            if (previewMode) {
                                setStatus("示例预览 · 已演示取消传输");
                            } else {
                                runTask("正在取消任务…", () -> {
                                    api.cancelTransfer(transfer);
                                    return "任务已取消";
                                }, true);
                            }
                        }).show());
                card.addView(cancel);
            }
            transferList.addView(card);
        }
    }

    private void renderDevices(List<DeviceInfo> devices) {
        List<DeviceInfo> snapshot = new ArrayList<>(devices);
        lastDevices.clear();
        lastDevices.addAll(snapshot);
        deviceList.removeAllViews();
        updateDeviceFilter(snapshot);
        if (snapshot.isEmpty()) {
            addEmptyState(deviceList, "尚未发现账号设备", "", "连接设置", view -> showSettings());
            return;
        }
        for (DeviceInfo device : snapshot) {
            LinearLayout row = new LinearLayout(this);
            row.setOrientation(LinearLayout.VERTICAL);
            styleCard(row);
            LinearLayout titleRow = new LinearLayout(this);
            titleRow.setOrientation(LinearLayout.HORIZONTAL);
            titleRow.setGravity(android.view.Gravity.CENTER_VERTICAL);
            TextView name = new TextView(this);
            name.setText(deviceDisplayName(device));
            name.setTextColor(getColor(R.color.ink));
            name.setTextSize(16);
            name.setTypeface(Typeface.DEFAULT, Typeface.BOLD);
            titleRow.addView(name, new LinearLayout.LayoutParams(0, LinearLayout.LayoutParams.WRAP_CONTENT, 1));
            titleRow.addView(coloredBadge(device.isOnline() ? "在线" : "离线",
                    device.isOnline() ? R.color.forest_dark : R.color.muted,
                    device.isOnline() ? R.color.success_bg : R.color.offline_bg));
            row.addView(titleRow);

            String activity = device.lastSeenAt.isEmpty() ? "尚无心跳" : "最近活动 " + displayTime(device.lastSeenAt);
            TextView meta = new TextView(this);
            meta.setText(device.platform + " · " + activity);
            meta.setTextColor(getColor(R.color.muted));
            meta.setTextSize(12);
            LinearLayout.LayoutParams metaParams = new LinearLayout.LayoutParams(LinearLayout.LayoutParams.MATCH_PARENT, LinearLayout.LayoutParams.WRAP_CONTENT);
            metaParams.topMargin = dp(5);
            row.addView(meta, metaParams);

            boolean lan = device.isOnline() && "lan".equals(device.connectionMode);
            TextView connection = coloredBadge(connectionLabel(device),
                    lan ? R.color.lan_text : device.isOnline() ? R.color.public_text : R.color.muted,
                    lan ? R.color.lan_bg : device.isOnline() ? R.color.public_bg : R.color.offline_bg);
            LinearLayout.LayoutParams connectionParams = new LinearLayout.LayoutParams(LinearLayout.LayoutParams.WRAP_CONTENT, LinearLayout.LayoutParams.WRAP_CONTENT);
            connectionParams.topMargin = dp(10);
            row.addView(connection, connectionParams);

            LinearLayout actions = new LinearLayout(this);
            actions.setOrientation(LinearLayout.HORIZONTAL);
            actions.setGravity(android.view.Gravity.END);
            Button files = new Button(this);
            files.setText("查看文件");
            styleSecondaryButton(files);
            files.setOnClickListener(v -> showDeviceFiles(device));
            actions.addView(files);
            Button manage = new Button(this);
            manage.setText("管理");
            styleSecondaryButton(manage);
            LinearLayout.LayoutParams manageParams = new LinearLayout.LayoutParams(LinearLayout.LayoutParams.WRAP_CONTENT, dp(44));
            manageParams.leftMargin = dp(8);
            manage.setLayoutParams(manageParams);
            manage.setOnClickListener(v -> showDeviceActions(device));
            actions.addView(manage);
            LinearLayout.LayoutParams actionsParams = new LinearLayout.LayoutParams(LinearLayout.LayoutParams.MATCH_PARENT, LinearLayout.LayoutParams.WRAP_CONTENT);
            actionsParams.topMargin = dp(10);
            row.addView(actions, actionsParams);
            deviceList.addView(row);
        }
    }

    private void updateDeviceFilter(List<DeviceInfo> devices) {
        String selected = activeDeviceFilter;
        deviceFilterIds.clear();
        deviceFilterIds.add("all");
        String[] labels = new String[devices.size() + 1];
        labels[0] = "全部设备";
        int selectedPosition = 0;
        for (int i = 0; i < devices.size(); i++) {
            DeviceInfo device = devices.get(i);
            deviceFilterIds.add(device.id);
            labels[i + 1] = deviceDisplayName(device);
            if (device.id.equals(selected)) selectedPosition = i + 1;
        }
        deviceFilterSpinner.setAdapter(compactSpinnerAdapter(labels));
        deviceFilterSpinner.setSelection(selectedPosition);
    }

    private String deviceDisplayName(DeviceInfo device) {
        String alias = deviceAliases.get(device.id);
        return alias == null || alias.isEmpty() ? device.name : alias;
    }

    private String connectionLabel(DeviceInfo device) {
        if (!device.isOnline()) return "未连接";
        if ("lan".equals(device.connectionMode)) {
            return "局域网";
        }
        return "公网";
    }

    private TextView coloredBadge(String text, int foreground, int background) {
        TextView badge = new TextView(this);
        badge.setText(text);
        badge.setTextColor(getColor(foreground));
        badge.setTextSize(12);
        badge.setTypeface(Typeface.DEFAULT, Typeface.BOLD);
        badge.setPadding(dp(10), dp(5), dp(10), dp(5));
        GradientDrawable shape = new GradientDrawable();
        shape.setColor(getColor(background));
        shape.setCornerRadius(dp(12));
        badge.setBackground(shape);
        return badge;
    }

    private void showDeviceFiles(DeviceInfo device) {
        activeDeviceFilter = device.id;
        int index = deviceFilterIds.indexOf(device.id);
        if (index >= 0) deviceFilterSpinner.setSelection(index);
        showSection(filesSection, filesTab);
        renderFiles(lastFiles);
    }

    private void showDeviceActions(DeviceInfo device) {
        Map<String, Runnable> actions = new LinkedHashMap<>();
        actions.put("查看设备文件", () -> showDeviceFiles(device));
        actions.put("重命名", () -> promptRenameDevice(device));
        actions.put("设备详情", () -> showDeviceDetails(device));
        if (!device.id.equals(api.currentDeviceId())) actions.put("移除设备", () -> confirmRemoveDevice(device));
        showActionSheet(deviceDisplayName(device), actions);
    }

    private void promptRenameDevice(DeviceInfo device) {
        EditText input = new EditText(this);
        input.setSingleLine(true);
        input.setText(deviceDisplayName(device));
        input.setSelection(input.length());
        new AlertDialog.Builder(this)
                .setTitle("重命名设备")
                .setView(input)
                .setNegativeButton("取消", null)
                .setPositiveButton("保存", (dialog, which) -> {
                    String name = input.getText().toString().trim();
                    if (name.isEmpty()) return;
                    deviceAliases.put(device.id, name);
                    renderDevices(lastDevices);
                })
                .show();
    }

    private void showDeviceDetails(DeviceInfo device) {
        String message = "状态：" + (device.isOnline() ? "在线" : "离线")
                + "\n连接模式：" + connectionLabel(device)
                + "\n平台：" + device.platform
                + "\n最近活动：" + (device.lastSeenAt.isEmpty() ? "尚无心跳" : displayTime(device.lastSeenAt))
                + "\n设备 ID：" + device.id;
        new AlertDialog.Builder(this).setTitle(deviceDisplayName(device)).setMessage(message)
                .setNegativeButton("关闭", null).setPositiveButton("查看文件", (dialog, which) -> showDeviceFiles(device)).show();
    }

    private void confirmRemoveDevice(DeviceInfo device) {
        new AlertDialog.Builder(this)
                .setTitle("移除设备？")
                .setMessage(deviceDisplayName(device) + " 的现有会话将失效")
                .setNegativeButton("取消", null)
                .setPositiveButton("移除", (dialog, which) -> {
                    if (previewMode) {
                        List<DeviceInfo> remaining = new ArrayList<>(lastDevices);
                        remaining.remove(device);
                        renderDevices(remaining);
                    } else {
                        runTask("正在移除设备…", () -> {
                            api.deregisterDevice(device);
                            return "已移除 " + deviceDisplayName(device);
                        }, true);
                    }
                }).show();
    }

    private void addDiscoveredAgent(String name, String url) {
        if (discoveredAgents.put(url, name) != null) return;
        Button choice = new Button(this);
        choice.setAllCaps(false);
        choice.setText(getString(R.string.discovered_agent_format, name, url));
        choice.setOnClickListener(v -> {
            agentUrl.setText(url);
            saveEndpoints();
            setStatus("已选择 " + name + "，登录后可访问；自动发现本身不授予权限");
            discovery.stop();
        });
        discoveryList.addView(choice);
    }

    private void renderFiles(List<LanFile> files) {
        List<LanFile> snapshot = new ArrayList<>(files);
        lastFiles.clear();
        lastFiles.addAll(snapshot);
        List<LanFile> visibleFiles = new ArrayList<>();
        for (LanFile file : snapshot) {
            if (matchesActiveFilter(file) && matchesDeviceFilter(file)) visibleFiles.add(file);
        }
        fileList.removeAllViews();
        String query = searchQuery.getText().toString().trim();
        fileSummary.setText(getString(query.isEmpty() && "all".equals(activeTypeFilter)
                ? R.string.file_count_format
                : R.string.file_match_count_format, visibleFiles.size()));
        if (visibleFiles.isEmpty()) {
            if (query.isEmpty() && "all".equals(activeTypeFilter)) {
                addEmptyState(fileList, "这里还没有文件", "", "上传文件", view -> chooseUpload());
            } else {
                addEmptyState(fileList, "没有找到匹配文件", "试试其他关键词或文件类型。", "重置筛选", view -> {
                    searchQuery.setText("");
                    selectFilter("all", true);
                });
            }
            return;
        }
        if (gridView) {
            GridLayout grid = new GridLayout(this);
            grid.setColumnCount(3);
            grid.setAlignmentMode(GridLayout.ALIGN_BOUNDS);
            for (LanFile file : visibleFiles) grid.addView(createGridFileItem(file));
            fileList.addView(grid);
        } else {
            for (LanFile file : visibleFiles) fileList.addView(createListFileItem(file));
        }
    }

    private void selectFilter(String filter, boolean rerender) {
        activeTypeFilter = filter;
        int position = "recent".equals(filter) ? 1
                : "images".equals(filter) ? 2
                : "documents".equals(filter) ? 3
                : "videos".equals(filter) ? 4 : 0;
        if (filterSpinner.getSelectedItemPosition() != position) filterSpinner.setSelection(position);
        if (rerender) renderFiles(lastFiles);
    }

    private boolean matchesActiveFilter(LanFile file) {
        switch (activeTypeFilter) {
        case "images": return file.mime.startsWith("image/");
        case "videos": return file.mime.startsWith("video/");
        case "documents":
            return !file.mime.startsWith("image/")
                    && !file.mime.startsWith("video/")
                    && !file.mime.startsWith("audio/")
                    && !file.mime.contains("zip")
                    && !file.mime.contains("compressed");
        case "recent":
            if (file.updatedAt.isEmpty()) return true;
            try {
                return Instant.parse(file.updatedAt).isAfter(Instant.now().minusSeconds(7 * 86400));
            } catch (Exception ignored) {
                return true;
            }
        default: return true;
        }
    }

    private boolean matchesDeviceFilter(LanFile file) {
        if ("all".equals(activeDeviceFilter)) return true;
        if (activeDeviceFilter.equals(file.originDeviceId)) return true;
        for (FileReplicaInfo replica : file.replicas) {
            if (activeDeviceFilter.equals(replica.deviceId)) return true;
        }
        return false;
    }

    private void updateViewModeButton() {
        int label = gridView ? R.string.list_view : R.string.grid_view;
        viewModeButton.setImageResource(gridView ? R.drawable.ic_list : R.drawable.ic_grid);
        viewModeButton.setContentDescription(getString(label));
    }

    private View createListFileItem(LanFile file) {
        LinearLayout row = new LinearLayout(this);
        row.setOrientation(LinearLayout.HORIZONTAL);
        row.setGravity(android.view.Gravity.CENTER_VERTICAL);
        row.setMinimumHeight(dp(76));
        row.setPadding(dp(4), dp(8), 0, dp(8));
        row.setBackgroundResource(R.drawable.file_row_background);
        row.setSelected(selectedFileIds.contains(file.id));

        TextView icon = fileIcon(file, 48);
        row.addView(icon, new LinearLayout.LayoutParams(dp(48), dp(48)));

        LinearLayout labels = new LinearLayout(this);
        labels.setOrientation(LinearLayout.VERTICAL);
        labels.setPadding(dp(12), 0, dp(4), 0);
        TextView title = fileTitle(file, 15, 1);
        labels.addView(title);
        TextView metadata = new TextView(this);
        metadata.setText(getString(R.string.file_metadata_format, formatBytes(file.size), shortTime(file.updatedAt)));
        metadata.setTextColor(getColor(R.color.muted));
        metadata.setTextSize(12);
        labels.addView(metadata);
        TextView availability = new TextView(this);
        availability.setText(availabilityLabel(file));
        availability.setTextColor(getColor(file.available ? R.color.forest_dark : R.color.muted));
        availability.setTextSize(12);
        labels.addView(availability);
        row.addView(labels, new LinearLayout.LayoutParams(0, LinearLayout.LayoutParams.WRAP_CONTENT, 1));

        Button more = fileMoreButton(file);
        row.addView(more, new LinearLayout.LayoutParams(dp(48), dp(48)));
        row.setOnClickListener(v -> {
            if (selectedFileIds.isEmpty()) showFileProperties(file); else toggleSelection(file);
        });
        row.setOnLongClickListener(v -> {
            toggleSelection(file);
            return true;
        });
        if (!file.available) row.setAlpha(0.48f);
        return row;
    }

    private View createGridFileItem(LanFile file) {
        LinearLayout item = new LinearLayout(this);
        item.setOrientation(LinearLayout.VERTICAL);
        item.setGravity(android.view.Gravity.CENTER_HORIZONTAL);
        item.setPadding(dp(8), dp(10), dp(8), dp(8));
        item.setBackgroundResource(R.drawable.file_grid_background);
        item.setSelected(selectedFileIds.contains(file.id));
        GridLayout.LayoutParams params = new GridLayout.LayoutParams();
        int availableWidth = getResources().getDisplayMetrics().widthPixels - dp(32) - dp(18);
        params.width = Math.max(dp(96), availableWidth / 3);
        params.height = GridLayout.LayoutParams.WRAP_CONTENT;
        params.columnSpec = GridLayout.spec(GridLayout.UNDEFINED);
        params.setMargins(dp(3), dp(3), dp(3), dp(3));
        item.setLayoutParams(params);
        item.addView(fileIcon(file, 56), new LinearLayout.LayoutParams(dp(56), dp(56)));
        TextView title = fileTitle(file, 13, 2);
        title.setGravity(android.view.Gravity.CENTER);
        LinearLayout.LayoutParams titleParams = new LinearLayout.LayoutParams(LinearLayout.LayoutParams.MATCH_PARENT, dp(40));
        titleParams.topMargin = dp(6);
        item.addView(title, titleParams);
        TextView metadata = new TextView(this);
        metadata.setText(formatBytes(file.size));
        metadata.setTextColor(getColor(R.color.muted));
        metadata.setTextSize(11);
        metadata.setGravity(android.view.Gravity.CENTER);
        item.addView(metadata, new LinearLayout.LayoutParams(LinearLayout.LayoutParams.MATCH_PARENT, LinearLayout.LayoutParams.WRAP_CONTENT));
        item.setOnClickListener(v -> {
            if (selectedFileIds.isEmpty()) showFileProperties(file); else toggleSelection(file);
        });
        item.setOnLongClickListener(v -> {
            toggleSelection(file);
            return true;
        });
        if (!file.available) item.setAlpha(0.48f);
        return item;
    }

    private TextView fileIcon(LanFile file, int sizeDp) {
        TextView icon = new TextView(this);
        icon.setText(fileGlyph(file.mime));
        icon.setTextColor(getColor(R.color.forest_dark));
        icon.setTextSize(sizeDp >= 56 ? 15 : 13);
        icon.setTypeface(Typeface.DEFAULT, Typeface.BOLD);
        icon.setGravity(android.view.Gravity.CENTER);
        icon.setBackgroundResource(R.drawable.file_icon_background);
        return icon;
    }

    private TextView fileTitle(LanFile file, int textSize, int maxLines) {
        TextView title = new TextView(this);
        title.setText(file.name);
        title.setTextColor(getColor(R.color.ink));
        title.setTextSize(textSize);
        title.setTypeface(Typeface.DEFAULT, Typeface.BOLD);
        title.setMaxLines(maxLines);
        title.setEllipsize(android.text.TextUtils.TruncateAt.MIDDLE);
        return title;
    }

    private Button fileMoreButton(LanFile file) {
        Button more = new Button(this);
        more.setText("⋮");
        more.setTextSize(22);
        more.setAllCaps(false);
        more.setContentDescription("打开 " + file.name + " 的操作菜单");
        more.setBackgroundColor(android.graphics.Color.TRANSPARENT);
        more.setTextColor(getColor(R.color.muted));
        more.setOnClickListener(v -> showFileActions(v, file));
        return more;
    }

    private String availabilityLabel(LanFile file) {
        if (!file.available) return "不可用 · 所有设备离线";
        int online = 0;
        for (FileReplicaInfo replica : file.replicas) if (replica.online) online++;
        if (online > 1) return "可用 · " + online + " 台设备在线";
        if (!file.originDeviceName.isEmpty()) return "可用 · " + file.originDeviceName;
        return "可用";
    }

    private String shortTime(String value) {
        if (value == null || value.isEmpty()) return "时间未知";
        try {
            long age = Math.max(0, Instant.now().getEpochSecond() - Instant.parse(value).getEpochSecond());
            if (age < 86400) return "今天";
            if (age < 172800) return "昨天";
            if (age < 7 * 86400) return (age / 86400) + " 天前";
        } catch (Exception ignored) {
            // Fall back to the server-provided date below.
        }
        return value.length() >= 10 ? value.substring(0, 10) : value;
    }

    private void toggleSelection(LanFile file) {
        if (!selectedFileIds.add(file.id)) selectedFileIds.remove(file.id);
        selectionBar.setVisibility(selectedFileIds.isEmpty() ? View.GONE : View.VISIBLE);
        selectionCount.setText(getString(R.string.selection_count_format, selectedFileIds.size()));
        renderFiles(lastFiles);
    }

    private void clearSelection() {
        selectedFileIds.clear();
        selectionBar.setVisibility(View.GONE);
        renderFiles(lastFiles);
    }

    private List<LanFile> selectedFiles() {
        List<LanFile> selected = new ArrayList<>();
        for (LanFile file : lastFiles) if (selectedFileIds.contains(file.id)) selected.add(file);
        return selected;
    }

    private void downloadSelection() {
        List<LanFile> selected = selectedFiles();
        if (selected.size() != 1) {
            setStatus("Android 需要为每个文件确认保存位置；请一次选择 1 个文件下载");
            return;
        }
        clearSelection();
        chooseDownload(selected.get(0));
    }

    private void shareSelection() {
        List<LanFile> selected = selectedFiles();
        if (selected.isEmpty()) return;
        if (previewMode) {
            clearSelection();
            setStatus("示例预览 · 已演示批量创建分享链接");
            return;
        }
        clearSelection();
        runTask("正在创建 " + selected.size() + " 个分享链接…", () -> {
            StringBuilder links = new StringBuilder();
            for (LanFile file : selected) {
                if (links.length() > 0) links.append('\n');
                links.append(file.name).append(": ").append(api.createShare(file));
            }
            runOnUiThread(() -> {
                ClipboardManager clipboard = (ClipboardManager) getSystemService(Context.CLIPBOARD_SERVICE);
                clipboard.setPrimaryClip(ClipData.newPlainText("Share Disk 分享链接", links.toString()));
            });
            return "已创建分享链接并复制到剪贴板";
        }, true);
    }

    private void trashSelection() {
        List<LanFile> selected = selectedFiles();
        if (selected.isEmpty()) return;
        new AlertDialog.Builder(this)
                .setTitle("移到回收站？")
                .setNegativeButton("取消", null)
                .setPositiveButton("移到回收站", (dialog, which) -> {
                    if (previewMode) {
                        clearSelection();
                        setStatus("示例预览 · 已演示批量移到回收站");
                        return;
                    }
                    clearSelection();
                    runTask("正在移动所选文件…", () -> {
                        for (LanFile file : selected) api.trashFile(file);
                        return "已将 " + selected.size() + " 个项目移到回收站";
                    }, true);
                })
                .show();
    }

    private void moveSelection() {
        List<LanFile> selected = selectedFiles();
        if (selected.isEmpty()) return;
        if (previewMode) {
            clearSelection();
            setStatus("示例预览 · 已演示批量移动文件");
            return;
        }
        setBusy(true);
        setStatus("正在读取文件夹…");
        executor.execute(() -> {
            try {
                List<FolderInfo> folders = api.listFolders();
                runOnUiThread(() -> {
                    setBusy(false);
                    String[] labels = new String[folders.size() + 1];
                    labels[0] = "全部文件（根目录）";
                    for (int i = 0; i < folders.size(); i++) labels[i + 1] = folders.get(i).name;
                    new AlertDialog.Builder(this)
                            .setTitle("移动 " + selected.size() + " 个项目")
                            .setItems(labels, (dialog, which) -> {
                                String folderId = which == 0 ? "" : folders.get(which - 1).id;
                                clearSelection();
                                runTask("正在移动所选文件…", () -> {
                                    for (LanFile file : selected) api.moveFile(file, folderId);
                                    return "已移动 " + selected.size() + " 个项目";
                                }, true);
                            })
                            .setNegativeButton("取消", null)
                            .show();
                });
            } catch (Exception error) {
                runOnUiThread(() -> {
                    setBusy(false);
                    setStatus("读取文件夹失败：" + safeMessage(error));
                });
            }
        });
    }

    private void showFileActions(View anchor, LanFile file) {
        Map<String, Runnable> actions = new LinkedHashMap<>();
        actions.put("文件详情", () -> showFileProperties(file));
        if (previewMode) {
            if (file.available) {
                actions.put("下载到本机", () -> chooseDownload(file));
                actions.put("发送到在线设备", () -> showPreviewReplication(file));
                actions.put("移到回收站", () -> confirmTrash(file));
            }
            showActionSheet(file.name, actions);
            return;
        }
        if (!file.available) {
            showActionSheet(file.name, actions);
            return;
        }
        actions.put("下载到本机", () -> chooseDownload(file));
        actions.put("创建 24 小时分享", () -> createShare(file));
        actions.put("移动到文件夹", () -> promptMove(file));
        actions.put("重命名", () -> promptRename(file));
        actions.put("发送到在线设备", () -> promptReplication(file));
        actions.put("移到回收站", () -> confirmTrash(file));
        showActionSheet(file.name, actions);
    }

    private void showActionSheet(String titleValue, Map<String, Runnable> actions) {
        Dialog dialog = new Dialog(this);
        LinearLayout sheet = new LinearLayout(this);
        sheet.setOrientation(LinearLayout.VERTICAL);
        sheet.setPadding(dp(20), dp(16), dp(20), dp(24));
        sheet.setBackgroundResource(R.drawable.action_sheet_background);

        TextView title = new TextView(this);
        title.setText(titleValue);
        title.setTextColor(getColor(R.color.ink));
        title.setTextSize(18);
        title.setTypeface(Typeface.DEFAULT, Typeface.BOLD);
        title.setPadding(dp(4), 0, dp(4), dp(10));
        sheet.addView(title);

        for (Map.Entry<String, Runnable> action : actions.entrySet()) {
            Button button = new Button(this);
            button.setText(action.getKey());
            button.setAllCaps(false);
            button.setGravity(android.view.Gravity.CENTER_VERTICAL | android.view.Gravity.START);
            button.setMinHeight(dp(52));
            button.setBackgroundColor(android.graphics.Color.TRANSPARENT);
            button.setTextColor(getColor(action.getKey().contains("回收站") ? R.color.danger : R.color.ink));
            button.setOnClickListener(v -> {
                dialog.dismiss();
                action.getValue().run();
            });
            sheet.addView(button, new LinearLayout.LayoutParams(LinearLayout.LayoutParams.MATCH_PARENT, dp(52)));
        }

        Button cancel = new Button(this);
        cancel.setText("取消");
        cancel.setAllCaps(false);
        cancel.setTextColor(getColor(R.color.muted));
        cancel.setBackgroundTintList(ColorStateList.valueOf(getColor(R.color.paper)));
        cancel.setOnClickListener(v -> dialog.dismiss());
        LinearLayout.LayoutParams cancelParams = new LinearLayout.LayoutParams(LinearLayout.LayoutParams.MATCH_PARENT, dp(48));
        cancelParams.topMargin = dp(8);
        sheet.addView(cancel, cancelParams);

        dialog.setContentView(sheet);
        android.view.Window window = dialog.getWindow();
        if (window != null) {
            window.setBackgroundDrawableResource(android.R.color.transparent);
            window.setLayout(android.view.WindowManager.LayoutParams.MATCH_PARENT, android.view.WindowManager.LayoutParams.WRAP_CONTENT);
            window.setGravity(android.view.Gravity.BOTTOM);
        }
        dialog.setOnShowListener(ignored -> {
            android.view.Window shownWindow = dialog.getWindow();
            if (shownWindow != null) shownWindow.setLayout(android.view.WindowManager.LayoutParams.MATCH_PARENT, android.view.WindowManager.LayoutParams.WRAP_CONTENT);
        });
        dialog.show();
    }

    private void showFileProperties(LanFile file) {
        StringBuilder message = new StringBuilder();
        message.append("大小：").append(String.format(Locale.US, "%,d 字节", file.size));
        message.append("\n类型：").append(file.mime);
        message.append("\n更新时间：").append(shortTime(file.updatedAt));
        message.append("\n上传设备：").append(file.originDeviceName.isEmpty() ? "未知设备" : file.originDeviceName);
        message.append("\n\n文件副本");
        if (file.replicas.isEmpty()) {
            message.append("\n暂无可用的副本信息");
        } else {
            for (FileReplicaInfo replica : file.replicas) {
                message.append("\n• ").append(replica.deviceName)
                        .append(replica.origin ? "（上传设备）" : "")
                        .append(" · ").append(replica.online ? "在线" : "离线")
                        .append(replica.platform.isEmpty() ? "" : " · " + replica.platform);
            }
        }
        message.append(file.available ? "\n\n至少一个在线副本可供下载。" : "\n\n所有副本设备均离线，文件暂不可操作。");

        AlertDialog.Builder dialog = new AlertDialog.Builder(this)
                .setTitle(file.name)
                .setMessage(message.toString())
                .setNegativeButton("关闭", null);
        if (file.available) {
            dialog.setNeutralButton("下载", (ignored, which) -> chooseDownload(file));
            dialog.setPositiveButton("发送到设备", (ignored, which) -> {
                if (previewMode) showPreviewReplication(file); else promptReplication(file);
            });
        }
        dialog.show();
    }

    private void showPreviewReplication(LanFile file) {
        String[] targets = {"家庭存储器 · 在线 · Ubuntu", "办公室电脑 · 离线（不可选）"};
        new AlertDialog.Builder(this)
                .setTitle("发送到设备")
                .setSingleChoiceItems(targets, 0, null)
                .setNegativeButton("取消", null)
                .setPositiveButton("下一步", (dialog, which) -> new AlertDialog.Builder(this)
                        .setTitle("确认发送文件？")
                        .setMessage("将通知“家庭存储器”主动下载并保存“" + file.name + "”。完成后该设备将拥有此文件副本。")
                        .setNegativeButton("取消", null)
                        .setPositiveButton("确认发送", (ignored, selected) -> setStatus("示例预览 · 已演示提交发送任务"))
                        .show())
                .show();
    }

    private void showSection(LinearLayout selected, View selectedTab) {
        currentSection = selected;
        settingsVisible = false;
        connectionPanel.setVisibility(View.GONE);
        filesSection.setVisibility(selected == filesSection ? View.VISIBLE : View.GONE);
        transfersSection.setVisibility(selected == transfersSection ? View.VISIBLE : View.GONE);
        devicesSection.setVisibility(selected == devicesSection ? View.VISIBLE : View.GONE);
        trashSection.setVisibility(selected == trashSection ? View.VISIBLE : View.GONE);
        filesTab.setSelected(selectedTab == filesTab);
        transfersTab.setSelected(selectedTab == transfersTab);
        devicesTab.setSelected(selectedTab == devicesTab);
        trashTab.setSelected(selectedTab == trashTab);
        if (selected == filesSection) screenTitle.setText(R.string.tab_files);
        if (selected == transfersSection) screenTitle.setText(R.string.tab_transfers);
        if (selected == devicesSection) screenTitle.setText(R.string.tab_devices);
        if (selected == trashSection) screenTitle.setText(R.string.tab_trash);
        globalRefreshButton.setVisibility(View.VISIBLE);
        uiHandler.removeCallbacks(transferPoll);
        if (selected == transfersSection) uiHandler.postDelayed(transferPoll, 1200);
    }

    private void showSectionByKey(String key) {
        switch (key) {
        case "transfers": showSection(transfersSection, transfersTab); break;
        case "devices": showSection(devicesSection, devicesTab); break;
        case "trash": showSection(trashSection, trashTab); break;
        default: showSection(filesSection, filesTab);
        }
    }

    private String currentSectionKey() {
        if (currentSection == transfersSection) return "transfers";
        if (currentSection == devicesSection) return "devices";
        if (currentSection == trashSection) return "trash";
        return "files";
    }

    private void restoreUiState(Bundle state) {
        previewMode = state.getBoolean(STATE_PREVIEW, false);
        selectedFolderId = state.getString(STATE_FOLDER, "");
        searchQuery.setText(state.getString(STATE_QUERY, ""));
        if (pendingSearch != null) {
            uiHandler.removeCallbacks(pendingSearch);
            pendingSearch = null;
        }
        sortSpinner.setSelection(state.getInt(STATE_SORT, 0));
        selectFilter(state.getString(STATE_FILTER, "all"), false);
        showSectionByKey(state.getString(STATE_SECTION, "files"));
        searchPanel.setVisibility(state.getBoolean(STATE_SEARCH_VISIBLE, false) && currentSection == filesSection
                ? View.VISIBLE : View.GONE);
        if (state.getBoolean(STATE_SETTINGS, false)) showSettings();
    }

    private void toggleSearch() {
        if (settingsVisible || currentSection != filesSection) {
            showSection(filesSection, filesTab);
        }
        boolean opening = searchPanel.getVisibility() != View.VISIBLE;
        searchPanel.setVisibility(opening ? View.VISIBLE : View.GONE);
        searchButton.setSelected(opening);
        InputMethodManager keyboard = (InputMethodManager) getSystemService(Context.INPUT_METHOD_SERVICE);
        if (opening) {
            searchQuery.requestFocus();
            searchQuery.post(() -> keyboard.showSoftInput(searchQuery, InputMethodManager.SHOW_IMPLICIT));
        } else {
            searchQuery.clearFocus();
            keyboard.hideSoftInputFromWindow(searchQuery.getWindowToken(), 0);
            if (searchQuery.length() > 0) searchQuery.setText("");
        }
    }

    private void showSettings() {
        settingsVisible = true;
        uiHandler.removeCallbacks(transferPoll);
        connectionPanel.setVisibility(View.VISIBLE);
        filesSection.setVisibility(View.GONE);
        transfersSection.setVisibility(View.GONE);
        devicesSection.setVisibility(View.GONE);
        trashSection.setVisibility(View.GONE);
        filesTab.setSelected(false);
        transfersTab.setSelected(false);
        devicesTab.setSelected(false);
        trashTab.setSelected(false);
        screenTitle.setText(R.string.settings);
        globalRefreshButton.setVisibility(View.GONE);
        setStatus(api.hasSession() ? "已登录 · 可修改连接信息" : "尚未连接 · 可登录或先查看示例界面");
    }

    private void returnToCurrentSection() {
        showSection(currentSection == null ? filesSection : currentSection,
                currentSection == transfersSection ? transfersTab : currentSection == devicesSection ? devicesTab : currentSection == trashSection ? trashTab : filesTab);
    }

    private void showPreview() {
        previewMode = true;
        try {
            List<LanFile> files = new ArrayList<>();
            files.add(new LanFile(new JSONObject("{\"id\":\"demo-1\",\"local_file_id\":\"demo-1\",\"name\":\"产品方案 v3.pdf\",\"mime\":\"application/pdf\",\"size\":5842031,\"updated_at\":\"2026-09-08T08:30:00Z\",\"sha256\":\"4f8c739afb8d1264f8c739afb8d1264f8c739afb8d1264f8c739afb8d1264\",\"origin_device_id\":\"nas\",\"origin_device_name\":\"家庭存储器\",\"available\":true,\"replicas\":[{\"device_id\":\"nas\",\"device_name\":\"家庭存储器\",\"platform\":\"Ubuntu\",\"state\":\"ready\",\"online\":true,\"is_origin\":true},{\"device_id\":\"work\",\"device_name\":\"办公室电脑\",\"platform\":\"Windows\",\"state\":\"ready\",\"online\":false,\"is_origin\":false}]}")));
            files.add(new LanFile(new JSONObject("{\"id\":\"demo-2\",\"local_file_id\":\"demo-2\",\"name\":\"云南旅行照片.zip\",\"mime\":\"application/zip\",\"size\":268435456,\"updated_at\":\"2026-09-07T12:15:00Z\",\"sha256\":\"19d7c2aa19d7c2aa19d7c2aa19d7c2aa19d7c2aa19d7c2aa19d7c2aa19d7c2aa\",\"origin_device_id\":\"phone\",\"origin_device_name\":\"我的手机\",\"available\":true,\"replicas\":[{\"device_id\":\"phone\",\"device_name\":\"我的手机\",\"platform\":\"Android\",\"state\":\"ready\",\"online\":true,\"is_origin\":true},{\"device_id\":\"nas\",\"device_name\":\"家庭存储器\",\"platform\":\"Ubuntu\",\"state\":\"ready\",\"online\":true,\"is_origin\":false}]}")));
            files.add(new LanFile(new JSONObject("{\"id\":\"demo-3\",\"local_file_id\":\"demo-3\",\"name\":\"毕业录像.mp4\",\"mime\":\"video/mp4\",\"size\":1288490188,\"updated_at\":\"2026-08-18T09:20:00Z\",\"sha256\":\"8a4b2d118a4b2d118a4b2d118a4b2d118a4b2d118a4b2d118a4b2d118a4b2d11\",\"origin_device_id\":\"old-pc\",\"origin_device_name\":\"旧笔记本\",\"available\":false,\"replicas\":[{\"device_id\":\"old-pc\",\"device_name\":\"旧笔记本\",\"platform\":\"Ubuntu\",\"state\":\"ready\",\"online\":false,\"is_origin\":true}]}")));
            applyPreviewFilters(files);

            String now = Instant.now().toString();
            List<DeviceInfo> devices = new ArrayList<>();
            devices.add(new DeviceInfo(new JSONObject("{\"id\":\"phone\",\"name\":\"我的手机\",\"platform\":\"Android\",\"status\":\"active\",\"connection_mode\":\"lan\",\"connected_peer\":\"家庭存储器\",\"last_seen_at\":\"" + now + "\"}")));
            devices.add(new DeviceInfo(new JSONObject("{\"id\":\"nas\",\"name\":\"家庭存储器\",\"platform\":\"Ubuntu\",\"status\":\"active\",\"connection_mode\":\"lan\",\"connected_peer\":\"我的手机\",\"last_seen_at\":\"" + now + "\"}")));
            devices.add(new DeviceInfo(new JSONObject("{\"id\":\"office\",\"name\":\"办公室电脑\",\"platform\":\"Windows\",\"status\":\"active\",\"connection_mode\":\"server\",\"last_seen_at\":\"" + now + "\"}")));
            devices.add(new DeviceInfo(new JSONObject("{\"id\":\"tablet\",\"name\":\"远程平板\",\"platform\":\"Android\",\"status\":\"active\",\"connection_mode\":\"server\",\"last_seen_at\":\"" + now + "\"}")));
            devices.add(new DeviceInfo(new JSONObject("{\"id\":\"old-pc\",\"name\":\"旧笔记本\",\"platform\":\"Ubuntu\",\"status\":\"active\",\"connection_mode\":\"offline\",\"last_seen_at\":\"2026-08-18T09:20:00Z\"}")));

            List<TransferInfo> transfers = new ArrayList<>();
            transfers.add(new TransferInfo(new JSONObject("{\"id\":\"transfer-1\",\"object_id\":\"产品方案 v3.pdf\",\"target_device_id\":\"nas\",\"state\":\"传输中 · 68%\",\"attempt\":1}")));
            List<FolderInfo> folders = new ArrayList<>();
            folders.add(new FolderInfo(new JSONObject("{\"id\":\"documents\",\"name\":\"文档\"}")));
            folders.add(new FolderInfo(new JSONObject("{\"id\":\"photos\",\"name\":\"照片\"}")));
            folders.add(new FolderInfo(new JSONObject("{\"id\":\"archive\",\"name\":\"归档\"}")));
            List<LanFile> trash = new ArrayList<>();
            trash.add(new LanFile(new JSONObject("{\"id\":\"trash-1\",\"name\":\"旧版报价单.xlsx\",\"mime\":\"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet\",\"size\":93621,\"sha256\":\"aa11\",\"status\":\"trashed\",\"deleted_at\":\"2026-09-05T10:30:00Z\",\"purge_after\":\"2026-09-12T10:30:00Z\",\"available\":true}")));

            renderFiles(files);
            renderDevices(devices);
            renderTransfers(transfers);
            renderFolders(folders);
            renderTrash(trash);
            renderShares(new ArrayList<>());
            renderBackgroundDownloads(new ArrayList<>());
            showSection(filesSection, filesTab);
            statusPanel.setVisibility(View.GONE);
        } catch (Exception error) {
            previewMode = false;
            setStatus("示例数据载入失败：" + safeMessage(error));
        }
    }

    private void createShare(LanFile file) {
        runTask("正在创建分享链接…", () -> {
            String link = api.createShare(file);
            runOnUiThread(() -> {
                TextView value = new TextView(this);
                value.setText(link);
                value.setTextIsSelectable(true);
                value.setPadding(dp(20), dp(12), dp(20), dp(12));
                new AlertDialog.Builder(this)
                        .setTitle("分享链接已创建")
                        .setMessage("链接有效 24 小时。访问后会得到文件信息、Agent 下载地址和短期下载令牌。")
                        .setView(value)
                        .setNegativeButton("关闭", null)
                        .setPositiveButton("复制", (dialog, which) -> {
                            ClipboardManager clipboard = (ClipboardManager) getSystemService(Context.CLIPBOARD_SERVICE);
                            clipboard.setPrimaryClip(ClipData.newPlainText("Share Disk link", link));
                            setStatus("分享链接已复制");
                        }).show();
            });
            return "分享链接已创建";
        }, false);
    }

    private void promptMove(LanFile file) {
        setBusy(true);
        executor.execute(() -> {
            try {
                List<FolderInfo> folders = api.listFolders();
                runOnUiThread(() -> {
                    setBusy(false);
                    String[] names = new String[folders.size() + 1];
                    names[0] = "全部文件（根目录）";
                    for (int i = 0; i < folders.size(); i++) names[i + 1] = folders.get(i).name;
                    new AlertDialog.Builder(this).setTitle("移动到").setItems(names, (dialog, which) -> runTask("正在移动…", () -> {
                        api.moveFile(file, which == 0 ? "" : folders.get(which - 1).id);
                        return "已移动 " + file.name;
                    }, true)).setNegativeButton("取消", null).show();
                });
            } catch (Exception error) {
                runOnUiThread(() -> { setBusy(false); setStatus("读取文件夹失败：" + safeMessage(error)); });
            }
        });
    }

    private void promptReplication(LanFile file) {
        setBusy(true);
        setStatus("正在读取可用设备…");
        executor.execute(() -> {
            try {
                List<DeviceInfo> all = api.listDevices();
                List<DeviceInfo> targets = new java.util.ArrayList<>();
                for (DeviceInfo device : all) {
                    if (isStoragePlatform(device.platform) && device.isOnline() && !device.id.equals(file.originDeviceId)) {
                        targets.add(device);
                    }
                }
                runOnUiThread(() -> {
                    setBusy(false);
                    if (targets.isEmpty()) {
                        setStatus("没有其他在线存储设备可作为发送目标");
                        return;
                    }
                    String[] labels = new String[targets.size()];
                    for (int i = 0; i < targets.size(); i++) {
                        DeviceInfo device = targets.get(i);
                        labels[i] = device.name + (device.lastSeenAt.isEmpty() ? " · 尚无心跳" : " · 最近在线 " + displayTime(device.lastSeenAt));
                    }
                    new AlertDialog.Builder(this)
                            .setTitle("选择复制目标")
                            .setItems(labels, (dialog, which) -> {
                                DeviceInfo target = targets.get(which);
                                confirmReplication(file, target);
                            })
                            .setNegativeButton("取消", null)
                            .show();
                    setStatus("请选择在线目标设备");
                });
            } catch (Exception error) {
                runOnUiThread(() -> {
                    setBusy(false);
                    setStatus("读取设备失败：" + safeMessage(error));
                });
            }
        });
    }

    private void confirmReplication(LanFile file, DeviceInfo target) {
        new AlertDialog.Builder(this)
                .setTitle("确认发送文件？")
                .setMessage("将通知“" + target.name + "”主动下载并保存“" + file.name + "”。完成后该设备将拥有此文件副本。")
                .setNegativeButton("取消", null)
                .setPositiveButton("确认发送", (dialog, which) -> runTask("正在提交复制任务…", () -> {
                    api.scheduleReplica(file, target);
                    return "已提交到 " + target.name + "；目标设备将下载、校验并登记副本";
                }, false))
                .show();
    }

    private void renderTrash(List<LanFile> files) {
        trashList.removeAllViews();
        if (files.isEmpty()) {
            addEmptyState(trashList, "回收站为空", "", null, null);
            return;
        }
        for (LanFile file : files) {
            LinearLayout card = new LinearLayout(this);
            card.setOrientation(LinearLayout.VERTICAL);
            styleCard(card);
            LinearLayout.LayoutParams params = new LinearLayout.LayoutParams(LinearLayout.LayoutParams.MATCH_PARENT, LinearLayout.LayoutParams.WRAP_CONTENT);
            params.bottomMargin = dp(10);
            card.setLayoutParams(params);

            TextView title = new TextView(this);
            title.setText(file.name);
            title.setTextColor(getColor(R.color.ink));
            title.setTextSize(17);
            title.setTypeface(Typeface.DEFAULT, Typeface.BOLD);
            card.addView(title);

            TextView details = new TextView(this);
            details.setText(getString(R.string.trash_details_format,
                    displayTime(file.deletedAt), displayTime(file.purgeAfter)));
            details.setTextColor(getColor(R.color.muted));
            details.setPadding(0, dp(5), 0, dp(8));
            card.addView(details);

            Button restore = new Button(this);
            restore.setText("恢复");
            restore.setEnabled(file.status.equals("trashed"));
            restore.setOnClickListener(v -> {
                if (previewMode) {
                    setStatus("示例预览 · 已演示恢复文件");
                } else {
                    runTask("正在恢复…", () -> {
                        api.restoreFile(file);
                        return "已恢复 " + file.name;
                    }, true);
                }
            });
            card.addView(restore);

            Button purge = new Button(this);
            purge.setText("永久删除");
            styleDangerButton(purge);
            purge.setOnClickListener(v -> confirmPurge(file));
            card.addView(purge);
            trashList.addView(card);
        }
    }

    private void promptRename(LanFile file) {
        EditText input = new EditText(this);
        input.setSingleLine(true);
        input.setText(file.name);
        input.selectAll();
        new AlertDialog.Builder(this)
                .setTitle("重命名")
                .setView(input)
                .setNegativeButton("取消", null)
                .setPositiveButton("保存", (dialog, which) -> {
                    String name = input.getText().toString().trim();
                    if (name.isEmpty()) {
                        setStatus("文件名不能为空");
                        return;
                    }
                    runTask("正在重命名…", () -> {
                    LanFile renamed = api.renameFile(file, name);
                    return "已重命名为 " + renamed.name;
                }, true);
                })
                .show();
    }

    private void confirmTrash(LanFile file) {
        new AlertDialog.Builder(this)
                .setTitle("移到回收站？")
                .setNegativeButton("取消", null)
                .setPositiveButton("移到回收站", (dialog, which) -> {
                    if (previewMode) {
                        setStatus("示例预览 · 已演示移到回收站");
                    } else {
                        runTask("正在删除…", () -> {
                            api.trashFile(file);
                            return "已移到回收站：" + file.name;
                        }, true);
                    }
                })
                .show();
    }

    private void confirmPurge(LanFile file) {
        new AlertDialog.Builder(this)
                .setTitle("永久删除？")
                .setNegativeButton("取消", null)
                .setPositiveButton("永久删除", (dialog, which) -> {
                    if (previewMode) {
                        setStatus("示例预览 · 已演示永久删除确认");
                    } else {
                        runTask("正在永久删除…", () -> {
                            api.purgeFile(file);
                            return "已永久删除：" + file.name;
                        }, true);
                    }
                })
                .show();
    }

    private static String displayTime(String value) {
        if (value == null || value.isEmpty()) return "未知";
        try {
            return DateTimeFormatter.ofPattern("MM-dd HH:mm")
                    .withZone(ZoneId.systemDefault())
                    .format(Instant.parse(value));
        } catch (RuntimeException ignored) {
            String compact = value.replace('T', ' ');
            return compact.length() > 16 ? compact.substring(0, 16) : compact;
        }
    }

    private void refreshBackgroundTransfersOnly() {
        if (settingsVisible || currentSection != transfersSection || isFinishing()) return;
        executor.execute(() -> {
            try {
                List<WorkInfo> work = new ArrayList<>();
                work.addAll(WorkManager.getInstance(this).getWorkInfosByTag(UploadWorker.TAG).get());
                work.addAll(WorkManager.getInstance(this).getWorkInfosByTag(DownloadWorker.TAG).get());
                runOnUiThread(() -> renderBackgroundDownloads(work));
            } catch (Exception ignored) {
                // The next poll or the explicit refresh button will retry.
            } finally {
                runOnUiThread(() -> {
                    if (!settingsVisible && currentSection == transfersSection && !isFinishing()) {
                        uiHandler.postDelayed(transferPoll, 2000);
                    }
                });
            }
        });
    }

    private static String workStateLabel(WorkInfo.State state) {
        switch (state) {
        case ENQUEUED: return "等待开始";
        case RUNNING: return "传输中";
        case SUCCEEDED: return "已完成";
        case FAILED: return "传输失败";
        case BLOCKED: return "等待网络";
        case CANCELLED: return "已取消";
        default: return state.name();
        }
    }

    private static String transferStateLabel(String state) {
        switch (state.toLowerCase(Locale.ROOT)) {
        case "queued": return "等待开始";
        case "running": case "transferring": return "传输中";
        case "completed": case "succeeded": return "已完成";
        case "cancelled": case "canceled": return "已取消";
        case "failed": case "failed_permanent": return "传输失败";
        default: return state;
        }
    }

    private void styleCard(LinearLayout card) {
        card.setBackgroundResource(R.drawable.panel);
        card.setPadding(dp(16), dp(14), dp(16), dp(14));
        card.setElevation(dp(1));
        LinearLayout.LayoutParams params = new LinearLayout.LayoutParams(
                LinearLayout.LayoutParams.MATCH_PARENT,
                LinearLayout.LayoutParams.WRAP_CONTENT);
        params.bottomMargin = dp(10);
        card.setLayoutParams(params);
    }

    private void styleDangerButton(Button button) {
        button.setAllCaps(false);
        button.setTextColor(getColor(R.color.danger));
        button.setBackgroundTintList(ColorStateList.valueOf(getColor(R.color.error_bg)));
        button.setMinHeight(dp(48));
    }

    private void styleSecondaryButton(Button button) {
        button.setAllCaps(false);
        button.setTextColor(getColor(R.color.forest_dark));
        button.setBackgroundTintList(ColorStateList.valueOf(getColor(R.color.mint)));
        button.setMinHeight(dp(44));
        button.setTextSize(13);
    }

    private void addEmptyState(LinearLayout parent, String titleValue, String detailValue, String actionLabel, View.OnClickListener action) {
        LinearLayout empty = new LinearLayout(this);
        empty.setOrientation(LinearLayout.VERTICAL);
        empty.setGravity(android.view.Gravity.CENTER_HORIZONTAL);
        empty.setPadding(dp(18), dp(28), dp(18), dp(28));
        empty.setBackgroundResource(R.drawable.empty_panel);

        TextView glyph = new TextView(this);
        glyph.setText("◇");
        glyph.setTextSize(30);
        glyph.setTextColor(getColor(R.color.forest));
        empty.addView(glyph);

        TextView title = new TextView(this);
        title.setText(titleValue);
        title.setTextColor(getColor(R.color.ink));
        title.setTextSize(16);
        title.setTypeface(Typeface.DEFAULT, Typeface.BOLD);
        title.setGravity(android.view.Gravity.CENTER);
        empty.addView(title);

        if (detailValue != null && !detailValue.isEmpty()) {
            TextView detail = new TextView(this);
            detail.setText(detailValue);
            detail.setTextColor(getColor(R.color.muted));
            detail.setTextSize(13);
            detail.setGravity(android.view.Gravity.CENTER);
            detail.setPadding(0, dp(6), 0, action == null ? 0 : dp(12));
            empty.addView(detail);
        }

        if (action != null && actionLabel != null) {
            Button button = new Button(this);
            button.setText(actionLabel);
            button.setAllCaps(false);
            button.setOnClickListener(action);
            button.setTextColor(getColor(R.color.forest_dark));
            button.setBackgroundTintList(ColorStateList.valueOf(getColor(R.color.mint)));
            LinearLayout.LayoutParams buttonParams = new LinearLayout.LayoutParams(LinearLayout.LayoutParams.WRAP_CONTENT, dp(48));
            buttonParams.topMargin = dp(12);
            empty.addView(button, buttonParams);
        }
        parent.addView(empty);
    }

    private void applyPreviewFilters(List<LanFile> files) {
        String query = searchQuery.getText().toString().trim().toLowerCase(Locale.ROOT);
        if (!query.isEmpty()) files.removeIf(file -> !file.name.toLowerCase(Locale.ROOT).contains(query));
        switch (selectedSort()) {
        case "name_desc": files.sort((left, right) -> right.name.compareToIgnoreCase(left.name)); break;
        case "size_desc": files.sort((left, right) -> Long.compare(right.size, left.size)); break;
        case "size_asc": files.sort((left, right) -> Long.compare(left.size, right.size)); break;
        default: files.sort((left, right) -> left.name.compareToIgnoreCase(right.name));
        }
    }

    private static String fileGlyph(String mime) {
        if (mime.startsWith("image/")) return "▧";
        if (mime.startsWith("video/")) return "▶";
        if (mime.contains("pdf")) return "PDF";
        if (mime.contains("zip") || mime.contains("compressed")) return "ZIP";
        if (mime.contains("spreadsheet") || mime.contains("excel")) return "XLS";
        return "DOC";
    }

    private static String friendlyType(String mime) {
        if (mime.startsWith("image/")) return "图片";
        if (mime.startsWith("video/")) return "视频";
        if (mime.contains("pdf")) return "PDF 文档";
        if (mime.contains("zip") || mime.contains("compressed")) return "压缩包";
        if (mime.contains("spreadsheet") || mime.contains("excel")) return "表格";
        return "文件";
    }

    private static boolean isStoragePlatform(String platform) {
        String value = platform.toLowerCase(Locale.ROOT);
        return value.contains("linux") || value.contains("ubuntu") || value.contains("windows");
    }

    private static String formatBytes(long value) {
        if (value >= 1024L * 1024 * 1024) return String.format(Locale.US, "%.1f GB", value / (1024d * 1024 * 1024));
        if (value >= 1024L * 1024) return String.format(Locale.US, "%.1f MB", value / (1024d * 1024));
        if (value >= 1024L) return String.format(Locale.US, "%.1f KB", value / 1024d);
        return value + " B";
    }

    private interface Task {
        String run() throws Exception;
    }

    private void runTask(String starting, Task task, boolean refreshAfter) {
        setBusy(true);
        setStatus(starting);
        executor.execute(() -> {
            try {
                String message = task.run();
                runOnUiThread(() -> {
                    if (destroyed) {
                        return;
                    }
                    setBusy(false);
                    setStatus(message);
                    if (refreshAfter) refreshFiles();
                });
            } catch (Exception error) {
                runOnUiThread(() -> {
                    if (destroyed) {
                        return;
                    }
                    setBusy(false);
                    setStatus("操作失败：" + safeMessage(error));
                });
            }
        });
    }

    private void saveEndpoints() {
        api.saveEndpoints(controlUrl.getText().toString(), agentUrl.getText().toString(), account.getText().toString());
    }

    private void setBusy(boolean busy) {
        this.busy = busy;
        statusProgress.setVisibility(busy ? View.VISIBLE : View.GONE);
        statusDismiss.setVisibility(busy ? View.GONE : View.VISIBLE);
        bootstrapButton.setEnabled(!busy);
        loginButton.setEnabled(!busy);
        logoutButton.setEnabled(!busy);
        uploadButton.setEnabled(!busy);
        refreshButton.setEnabled(!busy);
        globalRefreshButton.setEnabled(!busy);
        discoverButton.setEnabled(!busy);
        createFolderButton.setEnabled(!busy);
    }

    private void setStatusFromWorker(String value) {
        runOnUiThread(() -> setStatus(value));
    }

    private void setStatus(String value) {
        status.setText(value);
        String compact = value.replace('\n', ' ');
        if (compact.length() > 64) compact = compact.substring(0, 63) + "…";
        Toast toast = Toast.makeText(this, compact, Toast.LENGTH_SHORT);
        toast.setGravity(android.view.Gravity.BOTTOM | android.view.Gravity.CENTER_HORIZONTAL, 0, dp(118));
        toast.show();
    }

    private int dp(int value) {
        return Math.round(value * getResources().getDisplayMetrics().density);
    }

    private static String safeMessage(Exception error) {
        String message = error.getMessage();
        return message == null || message.isEmpty() ? error.getClass().getSimpleName() : message;
    }

    private static String prefix(String value, int length) {
        return value.length() <= length ? value : value.substring(0, length);
    }

    @Override
    protected void onDestroy() {
        destroyed = true;
        if (pendingSearch != null) uiHandler.removeCallbacks(pendingSearch);
        uiHandler.removeCallbacks(transferPoll);
        discovery.stop();
        executor.shutdownNow();
        super.onDestroy();
    }

    @Override
    protected void onSaveInstanceState(Bundle state) {
        state.putString(STATE_SECTION, currentSectionKey());
        state.putBoolean(STATE_SETTINGS, settingsVisible);
        state.putBoolean(STATE_PREVIEW, previewMode);
        state.putString(STATE_FOLDER, selectedFolderId);
        state.putString(STATE_QUERY, searchQuery.getText().toString());
        state.putInt(STATE_SORT, sortSpinner.getSelectedItemPosition());
        state.putString(STATE_FILTER, activeTypeFilter);
        state.putBoolean(STATE_SEARCH_VISIBLE, searchPanel.getVisibility() == View.VISIBLE);
        super.onSaveInstanceState(state);
    }

    @Override
    public void onBackPressed() {
        if (!selectedFileIds.isEmpty()) {
            clearSelection();
            return;
        }
        if (settingsVisible) {
            returnToCurrentSection();
            return;
        }
        super.onBackPressed();
    }
}
