"""Sinh cac so do cho bao cao do an tot nghiep.

Bao gom 10 hinh ve do phan giai cao, font ho tro tieng Viet co dau.
Chay: python3 docs/report-assets/generate-diagrams.py
"""
import matplotlib
matplotlib.use("Agg")
import matplotlib.pyplot as plt
import matplotlib.patches as patches
from matplotlib.patches import FancyBboxPatch, Ellipse, Rectangle
import os

# Font ho tro tieng Viet
matplotlib.rcParams["font.family"] = "DejaVu Sans"
matplotlib.rcParams["axes.unicode_minus"] = False

OUT = os.path.dirname(os.path.abspath(__file__))

C_BG       = "#FFFFFF"
C_CLIENT   = "#E8F1FF"
C_GATEWAY  = "#FFE9C7"
C_SERVICE  = "#D9EAD3"
C_INFRA    = "#F4CCCC"
C_STORE    = "#E1D5E7"
C_EDGE     = "#222222"
C_TEXT     = "#111111"
C_ACCENT   = "#0B2A52"


def box(ax, x, y, w, h, label, color, fontsize=9, bold=False):
    ax.add_patch(FancyBboxPatch(
        (x, y), w, h,
        boxstyle="round,pad=0.02,rounding_size=0.12",
        linewidth=1.3, edgecolor=C_EDGE, facecolor=color,
    ))
    ax.text(x + w / 2, y + h / 2, label,
            ha="center", va="center", fontsize=fontsize, color=C_TEXT,
            weight=("bold" if bold else "normal"))


def arrow(ax, x1, y1, x2, y2, label="", style="->", color=C_EDGE, fs=8, offset=(0, 0.08)):
    ax.annotate("", xy=(x2, y2), xytext=(x1, y1),
                arrowprops=dict(arrowstyle=style, color=color, lw=1.2,
                                shrinkA=3, shrinkB=3))
    if label:
        ax.text((x1 + x2) / 2 + offset[0], (y1 + y2) / 2 + offset[1], label,
                ha="center", va="center", fontsize=fs, color=C_TEXT,
                bbox=dict(facecolor="white", edgecolor="none", pad=0.8))


# =============================================================================
# HINH 1. KIEN TRUC TONG THE
# =============================================================================
fig, ax = plt.subplots(figsize=(13, 9), dpi=170)
ax.set_xlim(0, 14); ax.set_ylim(0, 11); ax.axis("off"); ax.set_facecolor(C_BG)

# Tier Client
box(ax, 0.3, 9.6, 13.4, 1.0,
    "Tầng Client — Web Browser (SPA) / Mobile App\nGiao thức: HTTPS (REST) + WebSocket",
    C_CLIENT, 10)

# Gateway
box(ax, 4.5, 8.0, 5.0, 1.1,
    "API Gateway (Gin + Reverse Proxy)\nXác thực JWT · Rate Limit · CORS · Routing · WS Hub",
    C_GATEWAY, 9.5)
arrow(ax, 7.0, 9.6, 7.0, 9.1, "REST + WS", offset=(0.6, 0))

# Services row
services = [
    (0.2, 6.0, 2.1, 1.4, "Auth\nService\n:8081\n(JWT · KYC · 2FA)"),
    (2.5, 6.0, 2.1, 1.4, "Wallet\nService\n:8082\n(VND · USDT · SePay)"),
    (4.8, 6.0, 2.1, 1.4, "Market\nService\n:8083\n(Bybit WS · Candles)"),
    (7.1, 6.0, 2.1, 1.4, "Trading\nService\n:8084\n(Spot · Matching)"),
    (9.4, 6.0, 2.1, 1.4, "Futures\nService\n:8085\n(Perp · Liquidation)"),
    (11.7, 6.0, 2.0, 1.4, "Notification\nService\n:8086"),
]
for (x, y, w, h, lb) in services:
    box(ax, x, y, w, h, lb, C_SERVICE, 8.5)
    arrow(ax, x + w / 2, 8.0, x + w / 2, 7.4)

ax.text(7.0, 5.75, "gRPC nội bộ (CheckBalance · Deduct · Lock · GetPrice · ValidateToken)",
        ha="center", fontsize=8.5, style="italic", color="#444")

# Infrastructure tier
box(ax, 0.3, 3.6, 4.5, 1.5,
    "Redis 7 (Hot-path)\n• Cache số dư (bal:userID:CUR, locked:userID:CUR)\n"
    "• Lua script atomic · Pub/Sub WS · Streams",
    C_INFRA, 9)
box(ax, 5.0, 3.6, 4.5, 1.5,
    "Apache Kafka 3.9\n(Event Backbone bền vững)\n"
    "Topics: trade.executed, balance.changed, order.updated,\nposition.changed, user.registered",
    C_INFRA, 9)
box(ax, 9.7, 3.6, 4.0, 1.5,
    "Elasticsearch 8.17\n(Index trades, orders)\n"
    "Phục vụ truy vấn lịch sử,\naudit và báo cáo",
    C_INFRA, 9)

for (x, y, w, h, _) in services:
    arrow(ax, x + w / 2, 6.0, x + w / 2, 5.1)

# Database tier
dbs = [
    (0.2, 1.2, 2.1, 1.6, "PostgreSQL\npg-auth\n:5551"),
    (2.5, 1.2, 2.1, 1.6, "PostgreSQL\npg-wallet\n:5552"),
    (4.8, 1.2, 2.1, 1.6, "PostgreSQL\npg-market\n:5553"),
    (7.1, 1.2, 2.1, 1.6, "PostgreSQL\npg-trading\n:5554"),
    (9.4, 1.2, 2.1, 1.6, "PostgreSQL\npg-futures\n:5555"),
    (11.7, 1.2, 2.0, 1.6, "PostgreSQL\npg-notification\n:5556"),
]
for (x, y, w, h, lb) in dbs:
    box(ax, x, y, w, h, lb, C_STORE, 8.5)
    arrow(ax, x + w / 2, 3.6, x + w / 2, 2.8)

ax.text(7.0, 0.55,
        "Nguyên tắc Database-per-Service — mỗi vi dịch vụ sở hữu một CSDL PostgreSQL 16 độc lập",
        ha="center", fontsize=9, style="italic", color="#333")

plt.title("Hình 1. Kiến trúc tổng thể nền tảng giao dịch Micro-Exchange",
          fontsize=13, fontweight="bold", pad=16, color=C_ACCENT)
plt.savefig(os.path.join(OUT, "fig1-architecture.png"),
            dpi=170, bbox_inches="tight", facecolor=C_BG)
plt.close()
print("OK fig1")


# =============================================================================
# HINH 2. LUONG DAT LENH SPOT (DATA FLOW)
# =============================================================================
fig, ax = plt.subplots(figsize=(13, 8), dpi=170)
ax.set_xlim(0, 14); ax.set_ylim(0, 10); ax.axis("off"); ax.set_facecolor(C_BG)

steps = [
    (0.3, 8.5, 2.6, 1.0, "1. Client\nPOST /api/trading/orders", C_CLIENT),
    (3.3, 8.5, 2.6, 1.0, "2. Gateway\n(Xác thực JWT)", C_GATEWAY),
    (6.3, 8.5, 3.0, 1.0, "3. Trading Handler\n(Validate payload)", C_SERVICE),
    (9.7, 8.5, 4.0, 1.0, "4. gRPC LockBalance\n→ Wallet Service", C_SERVICE),
    (9.7, 6.8, 4.0, 1.0, "5. MatchingEngine\n.ProcessOrder()", C_SERVICE),
    (6.3, 6.8, 3.0, 1.0, "6. OrderBook.Match\n(in-memory)", C_SERVICE),
    (3.3, 6.8, 2.6, 1.0, "7. Redis Lua\nupdateBalance (atomic)", C_INFRA),
    (0.3, 6.8, 2.6, 1.0, "8. PublishWS\ntrades@pair", C_INFRA),
    (0.3, 5.1, 2.6, 1.0, "9. Redis Streams\nstream:trade.executed", C_INFRA),
    (3.3, 5.1, 2.6, 1.0, "10. Kafka adapter\nfan-out", C_INFRA),
    (6.3, 5.1, 3.0, 1.0, "11. DB Projector\n(trading, wallet)", C_STORE),
    (9.7, 5.1, 4.0, 1.0, "12. ES Indexer\n(trades index)", C_STORE),
    (3.3, 3.4, 2.6, 1.0, "13. WebSocket Hub\nbroadcast depth", C_INFRA),
    (6.3, 3.4, 3.0, 1.0, "14. Notification\n(order filled)", C_SERVICE),
    (9.7, 3.4, 4.0, 1.0, "15. Client nhận\nupdate realtime", C_CLIENT),
]
for (x, y, w, h, lb, c) in steps:
    box(ax, x, y, w, h, lb, c, 8.5)

seq = [(0,1),(1,2),(2,3),(3,4),(4,5),(5,6),(6,7),(7,8),(8,9),(9,10),(9,11),(7,12),(12,13),(13,14)]
for a, b in seq:
    x1 = steps[a][0] + steps[a][2] / 2
    y1 = steps[a][1] + steps[a][3] / 2
    x2 = steps[b][0] + steps[b][2] / 2
    y2 = steps[b][1] + steps[b][3] / 2
    arrow(ax, x1, y1, x2, y2)

ax.text(7.0, 1.8,
        "Hot path: toàn bộ quá trình khớp lệnh và cập nhật số dư chỉ thao tác trên bộ nhớ + Redis.\n"
        "DB chỉ được ghi bất đồng bộ bởi projector sau khi consumer nhận sự kiện.",
        ha="center", fontsize=9.5, style="italic", color="#333")

plt.title("Hình 2. Luồng xử lý lệnh Spot theo mô hình CQRS (Command-Query Separation)",
          fontsize=12, fontweight="bold", pad=12, color=C_ACCENT)
plt.savefig(os.path.join(OUT, "fig2-spot-flow.png"),
            dpi=170, bbox_inches="tight", facecolor=C_BG)
plt.close()
print("OK fig2")


# =============================================================================
# HINH 3. LUONG FUTURES + LIQUIDATION
# =============================================================================
fig, ax = plt.subplots(figsize=(13, 8), dpi=170)
ax.set_xlim(0, 14); ax.set_ylim(0, 10); ax.axis("off"); ax.set_facecolor(C_BG)

fsteps = [
    (0.2, 8.5, 2.7, 1.0, "Client\nPOST /api/futures/order", C_CLIENT),
    (3.2, 8.5, 3.0, 1.0, "Futures Service\nOpenPosition()", C_SERVICE),
    (6.5, 8.5, 3.2, 1.0, "gRPC CheckBalance\n+ Deduct margin", C_SERVICE),
    (10.0, 8.5, 3.7, 1.0, "Tạo Position\n(pg-futures, OPEN)", C_STORE),
    (10.0, 6.8, 3.7, 1.0, "Publish\nposition.changed", C_INFRA),
    (6.5, 6.8, 3.2, 1.0, "Market Service\nprice:BTC_USDT (Redis)", C_INFRA),
    (3.2, 6.8, 3.0, 1.0, "Liquidation Engine\n(tick mỗi 1 giây)", C_SERVICE),
    (0.2, 6.8, 2.7, 1.0, "Kiểm tra\nPnL vs Margin", C_SERVICE),
    (0.2, 5.0, 2.7, 1.0, "PnL ≤ -100%?\n(LONG / SHORT)", C_INFRA),
    (3.2, 5.0, 3.0, 1.0, "ClosePosition\n(force-liquidate)", C_SERVICE),
    (6.5, 5.0, 3.2, 1.0, "Wallet.Credit\nsố dư còn lại", C_SERVICE),
    (10.0, 5.0, 3.7, 1.0, "Notification\nPOSITION_LIQUIDATED", C_SERVICE),
    (3.2, 3.3, 3.0, 1.0, "Publish\nbalance.changed", C_INFRA),
    (6.5, 3.3, 3.2, 1.0, "Projector ghi\npg-wallet + pg-futures", C_STORE),
    (10.0, 3.3, 3.7, 1.0, "WebSocket\n→ Client update", C_CLIENT),
]
for (x, y, w, h, lb, c) in fsteps:
    box(ax, x, y, w, h, lb, c, 8.5)

fseq = [(0,1),(1,2),(2,3),(3,4),(4,5),(5,6),(6,7),(7,8),(8,9),(9,10),(10,11),(11,12),(12,13),(13,14)]
for a, b in fseq:
    x1 = fsteps[a][0] + fsteps[a][2] / 2
    y1 = fsteps[a][1] + fsteps[a][3] / 2
    x2 = fsteps[b][0] + fsteps[b][2] / 2
    y2 = fsteps[b][1] + fsteps[b][3] / 2
    arrow(ax, x1, y1, x2, y2)

ax.text(7.0, 1.8,
        "Công thức giá thanh lý: LONG → EntryPrice × (1 − 1/leverage + 0.005);\n"
        "SHORT → EntryPrice × (1 + 1/leverage − 0.005). Maintenance margin 0,5%.",
        ha="center", fontsize=9.5, style="italic", color="#333")

plt.title("Hình 3. Luồng mở và thanh lý (Liquidation) vị thế Futures",
          fontsize=12, fontweight="bold", pad=12, color=C_ACCENT)
plt.savefig(os.path.join(OUT, "fig3-futures-flow.png"),
            dpi=170, bbox_inches="tight", facecolor=C_BG)
plt.close()
print("OK fig3")


# =============================================================================
# HINH 4. EVENT BUS FAN-OUT
# =============================================================================
fig, ax = plt.subplots(figsize=(13, 8), dpi=170)
ax.set_xlim(0, 14); ax.set_ylim(0, 10); ax.axis("off"); ax.set_facecolor(C_BG)

# Producers
producers = [
    (0.2, 7.8, 3.2, 1.2, "Trading Service\n(trade.executed, order.updated)"),
    (0.2, 6.2, 3.2, 1.2, "Wallet Service\n(balance.changed, deposit.confirmed)"),
    (0.2, 4.6, 3.2, 1.2, "Futures Service\n(position.changed)"),
    (0.2, 3.0, 3.2, 1.2, "Auth Service\n(user.registered, kyc.updated)"),
    (0.2, 1.4, 3.2, 1.2, "Market Service\n(price.updated)"),
]
for (x, y, w, h, lb) in producers:
    box(ax, x, y, w, h, lb, C_SERVICE, 9)

# Broker
box(ax, 4.0, 2.2, 5.0, 6.6,
    "Event Backbone\n\n"
    "Redis Streams (hot-path, ms latency)\n"
    "+\n"
    "Apache Kafka 3.9 (durable, replayable)\n\n"
    "Topics chính:\n"
    "• stream:trade.executed\n"
    "• stream:order.updated\n"
    "• stream:balance.changed\n"
    "• stream:position.changed\n"
    "• stream:user.registered\n"
    "• stream:price.updated",
    C_INFRA, 9.5)

# Consumers
consumers = [
    (9.6, 7.8, 4.0, 1.2, "ES Indexer\n(Elasticsearch)"),
    (9.6, 6.2, 4.0, 1.2, "DB Projector\n(PostgreSQL sync)"),
    (9.6, 4.6, 4.0, 1.2, "Notification Service\n(push / email / in-app)"),
    (9.6, 3.0, 4.0, 1.2, "Analytics / Audit\n(báo cáo, KPI, BI)"),
    (9.6, 1.4, 4.0, 1.2, "WebSocket Hub\n(realtime fan-out)"),
]
for (x, y, w, h, lb) in consumers:
    box(ax, x, y, w, h, lb, C_STORE, 9)

for (x, y, w, h, _) in producers:
    arrow(ax, x + w, y + h / 2, 4.0, y + h / 2)
for (x, y, w, h, _) in consumers:
    arrow(ax, 9.0, y + h / 2, x, y + h / 2)

ax.text(7.0, 0.5,
        "Mỗi consumer group xử lý độc lập, cho phép scale ngang và replay sự kiện từ offset bất kỳ.",
        ha="center", fontsize=9, style="italic", color="#333")

plt.title("Hình 4. Sơ đồ Event Bus — Producer · Broker · Consumer",
          fontsize=12, fontweight="bold", pad=12, color=C_ACCENT)
plt.savefig(os.path.join(OUT, "fig4-event-bus.png"),
            dpi=170, bbox_inches="tight", facecolor=C_BG)
plt.close()
print("OK fig4")


# =============================================================================
# HINH 5. USE CASE DIAGRAM
# =============================================================================
fig, ax = plt.subplots(figsize=(13, 9), dpi=170)
ax.set_xlim(0, 14); ax.set_ylim(0, 11); ax.axis("off"); ax.set_facecolor(C_BG)

def actor(ax, x, y, label):
    circle = plt.Circle((x, y + 0.45), 0.2, color=C_EDGE, fill=False, lw=1.6)
    ax.add_patch(circle)
    ax.plot([x, x], [y + 0.25, y - 0.28], color=C_EDGE, lw=1.6)
    ax.plot([x - 0.25, x + 0.25], [y + 0.02, y + 0.02], color=C_EDGE, lw=1.6)
    ax.plot([x, x - 0.22], [y - 0.28, y - 0.62], color=C_EDGE, lw=1.6)
    ax.plot([x, x + 0.22], [y - 0.28, y - 0.62], color=C_EDGE, lw=1.6)
    ax.text(x, y - 0.95, label, ha="center", fontsize=10, fontweight="bold")

actor(ax, 0.9, 7.5, "Người dùng")
actor(ax, 0.9, 3.0, "Quản trị viên")

# System boundary
ax.add_patch(patches.FancyBboxPatch((2.8, 0.4), 11.0, 10.3,
    boxstyle="round,pad=0.05,rounding_size=0.2",
    linewidth=1.6, edgecolor=C_EDGE, facecolor="#FAFAFA"))
ax.text(8.3, 10.4, "Nền tảng Micro-Exchange",
        ha="center", fontsize=12, fontweight="bold", color=C_ACCENT)

user_ucs = [
    (5.0, 9.5, "Đăng ký / Đăng nhập"),
    (10.0, 9.5, "Xác thực 2FA (TOTP)"),
    (5.0, 8.3, "Nộp hồ sơ KYC"),
    (10.0, 8.3, "Nạp VND qua SePay QR"),
    (5.0, 7.1, "Nạp USDT on-chain"),
    (10.0, 7.1, "Xem bảng giá, biểu đồ nến"),
    (5.0, 5.9, "Đặt lệnh Spot (LIMIT / MARKET)"),
    (10.0, 5.9, "Huỷ lệnh chờ khớp"),
    (5.0, 4.7, "Mở / đóng vị thế Futures"),
    (10.0, 4.7, "Xem lịch sử giao dịch"),
    (5.0, 3.5, "Nhận thông báo realtime"),
    (10.0, 3.5, "Rút tiền về ngân hàng"),
]
admin_ucs = [
    (5.0, 2.3, "Duyệt hồ sơ KYC"),
    (10.0, 2.3, "Cấu hình phí giao dịch"),
    (5.0, 1.1, "Duyệt yêu cầu rút tiền"),
    (10.0, 1.1, "Giám sát nhật ký gian lận"),
]

for (x, y, lb) in user_ucs + admin_ucs:
    e = Ellipse((x, y), 3.8, 0.9, facecolor=C_SERVICE, edgecolor=C_EDGE, lw=1.2)
    ax.add_patch(e)
    ax.text(x, y, lb, ha="center", va="center", fontsize=9)

for (x, y, _) in user_ucs:
    ax.plot([1.3, x - 1.95], [7.45, y], color="#666", lw=0.9)
for (x, y, _) in admin_ucs:
    ax.plot([1.3, x - 1.95], [2.95, y], color="#666", lw=0.9)

plt.title("Hình 5. Biểu đồ Use Case tổng hợp của hệ thống",
          fontsize=12, fontweight="bold", pad=12, color=C_ACCENT)
plt.savefig(os.path.join(OUT, "fig5-usecase.png"),
            dpi=170, bbox_inches="tight", facecolor=C_BG)
plt.close()
print("OK fig5")


# =============================================================================
# HINH 6. SEQUENCE DIAGRAM — PLACE SPOT ORDER
# =============================================================================
fig, ax = plt.subplots(figsize=(14, 10), dpi=170)
ax.set_xlim(0, 14); ax.set_ylim(0, 12); ax.axis("off"); ax.set_facecolor(C_BG)

# Lifelines
lifelines = [
    (1.2, "Client"),
    (3.0, "Gateway"),
    (4.8, "Trading\nHandler"),
    (6.6, "Wallet\nService"),
    (8.4, "Matching\nEngine"),
    (10.2, "Redis\n(Lua)"),
    (12.0, "Event\nBus"),
    (13.5, "DB /\nProjector"),
]
for (x, name) in lifelines:
    # header box
    box(ax, x - 0.65, 10.9, 1.3, 0.8, name, C_SERVICE, 9, bold=True)
    # dashed lifeline
    ax.plot([x, x], [0.5, 10.9], linestyle="--", color="#888", lw=0.9)

# Messages (from_x, to_x, y, label)
messages = [
    (1.2,  3.0, 10.4, "1. POST /api/trading/orders {pair, side, type, price, amount}"),
    (3.0,  4.8, 9.9,  "2. Forward (đã xác thực JWT)"),
    (4.8,  6.6, 9.4,  "3. gRPC LockBalance(userID, currency, amount)"),
    (6.6,  4.8, 8.9,  "4. OK (success)"),
    (4.8,  8.4, 8.4,  "5. ProcessOrder(order)"),
    (8.4,  8.4, 7.9,  "6. OrderBook.Match() — thuật toán price-time priority"),
    (8.4, 10.2, 7.4,  "7. BalanceDeduct / Credit / Unlock (Lua atomic)"),
    (10.2, 8.4, 6.9,  "8. OK (new balance)"),
    (8.4, 12.0, 6.4,  "9. Publish trade.executed, balance.changed, order.updated"),
    (12.0,13.5, 5.9,  "10. Consumer → INSERT vào bảng trades, wallets"),
    (8.4,  1.2, 5.4,  "11. WebSocket broadcast depth@pair, trades@pair"),
    (8.4,  4.8, 4.9,  "12. Trả về danh sách trade đã khớp"),
    (4.8,  3.0, 4.4,  "13. HTTP 200 {orderId, status, trades}"),
    (3.0,  1.2, 3.9,  "14. Trả kết quả"),
]
for (x1, x2, y, lb) in messages:
    arrow(ax, x1, y, x2, y)
    ax.text((x1 + x2) / 2, y + 0.12, lb, ha="center", fontsize=8, color=C_TEXT,
            bbox=dict(facecolor="white", edgecolor="none", pad=0.8))

ax.text(7.0, 0.1,
        "Tất cả mũi tên ngang là message; các thanh dọc đứt là lifeline của đối tượng tương ứng.",
        ha="center", fontsize=9, style="italic", color="#555")

plt.title("Hình 6. Sơ đồ tuần tự (Sequence Diagram) — Đặt lệnh giao dịch Spot",
          fontsize=12, fontweight="bold", pad=12, color=C_ACCENT)
plt.savefig(os.path.join(OUT, "fig6-seq-spot.png"),
            dpi=170, bbox_inches="tight", facecolor=C_BG)
plt.close()
print("OK fig6")


# =============================================================================
# HINH 7. SEQUENCE DIAGRAM — LIQUIDATION
# =============================================================================
fig, ax = plt.subplots(figsize=(14, 9), dpi=170)
ax.set_xlim(0, 14); ax.set_ylim(0, 11); ax.axis("off"); ax.set_facecolor(C_BG)

ll = [
    (1.3, "Liquidation\nEngine"),
    (3.3, "Redis\n(Price feed)"),
    (5.3, "Position\nRepo"),
    (7.3, "Wallet\nService"),
    (9.3, "Event\nBus"),
    (11.3, "Notification\nService"),
    (13.0, "Client\n(WebSocket)"),
]
for (x, name) in ll:
    box(ax, x - 0.7, 9.8, 1.4, 0.8, name, C_SERVICE, 9, bold=True)
    ax.plot([x, x], [0.5, 9.8], linestyle="--", color="#888", lw=0.9)

msgs = [
    (1.3,  3.3, 9.3, "1. Mỗi 1s: GET price:BTC_USDT"),
    (3.3,  1.3, 8.9, "2. Trả về markPrice"),
    (1.3,  5.3, 8.5, "3. FindOpenPositions() — lấy tất cả OPEN"),
    (5.3,  1.3, 8.1, "4. Danh sách Position"),
    (1.3,  1.3, 7.7, "5. Tính PnL = Size × (mark − entry) [LONG] / (entry − mark) [SHORT]"),
    (1.3,  1.3, 7.3, "6. Nếu PnL ≤ -(Margin × 0.995) → thanh lý"),
    (1.3,  5.3, 6.9, "7. UpdateStatus(LIQUIDATED, closedAt=now)"),
    (1.3,  7.3, 6.5, "8. gRPC Credit(userID, USDT, remaining = Margin + PnL)"),
    (7.3,  1.3, 6.1, "9. OK"),
    (1.3,  9.3, 5.7, "10. Publish position.changed, balance.changed"),
    (9.3, 11.3, 5.3, "11. Handle NotificationEvent(POSITION_LIQUIDATED)"),
    (11.3,13.0, 4.9, "12. Push realtime notification tới client"),
    (1.3, 13.0, 4.5, "13. WebSocket broadcast positions@user"),
]
for (x1, x2, y, lb) in msgs:
    arrow(ax, x1, y, x2, y)
    ax.text((x1 + x2) / 2, y + 0.15, lb, ha="center", fontsize=8, color=C_TEXT,
            bbox=dict(facecolor="white", edgecolor="none", pad=0.8))

plt.title("Hình 7. Sơ đồ tuần tự — Thanh lý vị thế Futures (Liquidation Engine)",
          fontsize=12, fontweight="bold", pad=12, color=C_ACCENT)
plt.savefig(os.path.join(OUT, "fig7-seq-liquidation.png"),
            dpi=170, bbox_inches="tight", facecolor=C_BG)
plt.close()
print("OK fig7")


# =============================================================================
# HINH 8. ERD TONG HOP
# =============================================================================
fig, ax = plt.subplots(figsize=(14, 10), dpi=170)
ax.set_xlim(0, 16); ax.set_ylim(0, 11); ax.axis("off"); ax.set_facecolor(C_BG)

def entity(ax, x, y, name, fields, color=C_STORE, w=3.0):
    h = 0.5 + 0.35 * len(fields)
    ax.add_patch(Rectangle((x, y), w, h, facecolor=color, edgecolor=C_EDGE, lw=1.3))
    ax.add_patch(Rectangle((x, y + h - 0.5), w, 0.5, facecolor="#0B2A52", edgecolor=C_EDGE, lw=1.3))
    ax.text(x + w / 2, y + h - 0.25, name, ha="center", va="center",
            fontsize=10, color="white", fontweight="bold")
    for i, f in enumerate(fields):
        ax.text(x + 0.15, y + h - 0.8 - i * 0.35, f, fontsize=8, va="center")
    return (x, y, w, h)

# Auth DB
e1 = entity(ax, 0.3, 7.8, "users (pg-auth)",
            ["id PK", "email UNIQUE", "password_hash", "role",
             "is_2fa", "kyc_status", "kyc_step", "is_locked"])
e2 = entity(ax, 0.3, 4.6, "kyc_profiles",
            ["id PK", "user_id FK", "first_name", "last_name",
             "date_of_birth", "address", "occupation"])
e3 = entity(ax, 0.3, 1.4, "kyc_documents",
            ["id PK", "user_id FK", "doc_type", "file_path", "status"])
e4 = entity(ax, 4.0, 1.4, "fraud_logs",
            ["id PK", "user_ids", "fraud_type", "action"])

# Wallet DB
e5 = entity(ax, 4.0, 7.8, "wallets (pg-wallet)",
            ["id PK", "user_id", "currency", "balance",
             "locked_balance", "updated_at"])
e6 = entity(ax, 4.0, 4.6, "deposits",
            ["id PK", "user_id", "amount", "method",
             "status", "order_code", "sepay_ref"])
e7 = entity(ax, 7.7, 4.6, "withdrawals",
            ["id PK", "user_id", "amount", "bank_code",
             "bank_account", "status"])

# Trading DB
e8 = entity(ax, 7.7, 7.8, "orders (pg-trading)",
            ["id PK", "user_id", "pair", "side", "type",
             "price", "amount", "filled_amount", "status"])
e9 = entity(ax, 11.4, 7.8, "trades",
            ["id PK", "pair", "buy_order_id", "sell_order_id",
             "price", "amount", "buyer_fee", "seller_fee"])

# Futures DB
e10 = entity(ax, 11.4, 4.6, "futures_positions\n(pg-futures)",
            ["id PK", "user_id", "pair", "side",
             "leverage", "entry_price", "size",
             "margin", "liquidation_price", "status"])

# Market DB
e11 = entity(ax, 7.7, 1.4, "candles (pg-market)",
            ["pair+interval+open_time PK",
             "open", "high", "low", "close", "volume"])

# Notification DB
e12 = entity(ax, 11.4, 1.4, "notifications\n(pg-notification)",
            ["id PK", "user_id", "type", "title",
             "message", "is_read"])

# Relationships (dashed logical)
def rel(a, b, label=""):
    (ax1, ay1, aw, ah) = a
    (ax2, ay2, bw, bh) = b
    x1 = ax1 + aw
    y1 = ay1 + ah / 2
    x2 = ax2
    y2 = ay2 + bh / 2
    ax.annotate("", xy=(x2, y2), xytext=(x1, y1),
                arrowprops=dict(arrowstyle="-|>", color="#555", lw=1, linestyle="dashed"))

rel(e1, e2, "1-1")
rel(e1, e3, "1-N")
rel(e1, e5, "1-N (logical, via event)")
rel(e1, e8, "1-N")
rel(e1, e10, "1-N")
rel(e5, e6, "1-N")
rel(e5, e7, "1-N")
rel(e8, e9, "1-N")
rel(e1, e12, "1-N")
rel(e1, e4, "1-N")

ax.text(8.0, 0.25,
        "Chú thích: Các liên kết liên-CSDL (vd users ↔ wallets) là quan hệ logic — được duy trì qua sự kiện user.registered, không có khoá ngoại vật lý.",
        ha="center", fontsize=8.5, style="italic", color="#444")

plt.title("Hình 8. Sơ đồ quan hệ thực thể (ERD) tổng hợp — Database-per-Service",
          fontsize=12, fontweight="bold", pad=12, color=C_ACCENT)
plt.savefig(os.path.join(OUT, "fig8-erd.png"),
            dpi=170, bbox_inches="tight", facecolor=C_BG)
plt.close()
print("OK fig8")


# =============================================================================
# HINH 9. DEPLOYMENT DIAGRAM
# =============================================================================
fig, ax = plt.subplots(figsize=(14, 9), dpi=170)
ax.set_xlim(0, 14); ax.set_ylim(0, 10); ax.axis("off"); ax.set_facecolor(C_BG)

# Host / cluster nodes
ax.add_patch(patches.FancyBboxPatch((0.2, 0.5), 13.6, 9.0,
    boxstyle="round,pad=0.05,rounding_size=0.25",
    linewidth=1.5, edgecolor="#333", facecolor="#F7F7F7"))
ax.text(7.0, 9.25, "Docker Host (docker-compose up)",
        ha="center", fontsize=12, fontweight="bold", color=C_ACCENT)

# Service containers row
scs = [
    (0.5, 6.6, 2.1, 1.4, "auth-service\nport 8081 / grpc 9081"),
    (2.8, 6.6, 2.1, 1.4, "wallet-service\nport 8082 / grpc 9082"),
    (5.1, 6.6, 2.1, 1.4, "market-service\nport 8083 / grpc 9083"),
    (7.4, 6.6, 2.1, 1.4, "trading-service\nport 8084"),
    (9.7, 6.6, 2.1, 1.4, "futures-service\nport 8085"),
    (12.0, 6.6, 1.7, 1.4, "notification\nport 8086"),
]
for (x, y, w, h, lb) in scs:
    box(ax, x, y, w, h, lb, C_SERVICE, 8.5)

# Gateway + es-indexer
box(ax, 0.5, 8.3, 4.0, 0.8, "gateway (port 8080 — entry point)", C_GATEWAY, 9)
box(ax, 4.7, 8.3, 3.0, 0.8, "es-indexer (worker)", C_GATEWAY, 9)

# Infra row
infra = [
    (0.5, 4.6, 2.3, 1.4, "Redis 7\n:6389\n(cache + streams)"),
    (3.0, 4.6, 2.3, 1.4, "Kafka 3.9\n:9192\n(event backbone)"),
    (5.5, 4.6, 2.3, 1.4, "Elasticsearch\n8.17 :9201"),
]
for (x, y, w, h, lb) in infra:
    box(ax, x, y, w, h, lb, C_INFRA, 9)

# DB row
dbrow = [
    (0.5, 2.2, 2.1, 1.8, "pg-auth\n:5551"),
    (2.8, 2.2, 2.1, 1.8, "pg-wallet\n:5552"),
    (5.1, 2.2, 2.1, 1.8, "pg-market\n:5553"),
    (7.4, 2.2, 2.1, 1.8, "pg-trading\n:5554"),
    (9.7, 2.2, 2.1, 1.8, "pg-futures\n:5555"),
    (12.0, 2.2, 1.7, 1.8, "pg-notif\n:5556"),
]
for (x, y, w, h, lb) in dbrow:
    box(ax, x, y, w, h, lb, C_STORE, 9)

ax.text(7.0, 1.4,
        "Tất cả container nằm trên mạng Docker nội bộ. Volume riêng cho mỗi PostgreSQL, healthcheck tự động.",
        ha="center", fontsize=9, style="italic", color="#333")

plt.title("Hình 9. Sơ đồ triển khai (Deployment Diagram) — Docker Compose",
          fontsize=12, fontweight="bold", pad=12, color=C_ACCENT)
plt.savefig(os.path.join(OUT, "fig9-deployment.png"),
            dpi=170, bbox_inches="tight", facecolor=C_BG)
plt.close()
print("OK fig9")


# =============================================================================
# HINH 10. ORDER BOOK STRUCTURE + MATCHING ALGO
# =============================================================================
fig, ax = plt.subplots(figsize=(14, 8), dpi=170)
ax.set_xlim(0, 14); ax.set_ylim(0, 10); ax.axis("off"); ax.set_facecolor(C_BG)

# Bids
ax.text(3.5, 9.5, "BIDS — Lệnh MUA (sắp xếp giá DESC)",
        ha="center", fontsize=11, fontweight="bold", color="#0B6B2E")
# Asks
ax.text(10.5, 9.5, "ASKS — Lệnh BÁN (sắp xếp giá ASC)",
        ha="center", fontsize=11, fontweight="bold", color="#A91B1B")

# Bid levels
bid_levels = [
    (2.0, 8.2, 3.0, 0.8, "Price 65,100 USDT — [U12: 0.5, U45: 1.2]"),
    (2.0, 7.3, 3.0, 0.8, "Price 65,050 USDT — [U7: 2.0]"),
    (2.0, 6.4, 3.0, 0.8, "Price 65,000 USDT — [U19: 3.5, U23: 0.1]"),
    (2.0, 5.5, 3.0, 0.8, "Price 64,950 USDT — [U3: 0.8]"),
    (2.0, 4.6, 3.0, 0.8, "Price 64,900 USDT — [U31: 5.2]"),
]
for (x, y, w, h, lb) in bid_levels:
    box(ax, x, y, w, h, lb, "#D9F0D9", 9)

# Ask levels
ask_levels = [
    (9.0, 8.2, 3.0, 0.8, "Price 65,200 USDT — [U8: 0.9]"),
    (9.0, 7.3, 3.0, 0.8, "Price 65,250 USDT — [U11: 1.5, U22: 0.4]"),
    (9.0, 6.4, 3.0, 0.8, "Price 65,300 USDT — [U14: 2.3]"),
    (9.0, 5.5, 3.0, 0.8, "Price 65,350 USDT — [U5: 1.0]"),
    (9.0, 4.6, 3.0, 0.8, "Price 65,400 USDT — [U17: 4.8]"),
]
for (x, y, w, h, lb) in ask_levels:
    box(ax, x, y, w, h, lb, "#F0D9D9", 9)

# Incoming order
box(ax, 5.5, 2.8, 4.0, 1.0,
    "Lệnh tới (BUY MARKET 2.5 BTC)", "#FFF2CC", 10, bold=True)
arrow(ax, 7.5, 2.8, 10.5, 4.6, "")
arrow(ax, 7.5, 2.8, 10.5, 5.5, "")

# Pseudocode box
ax.add_patch(Rectangle((0.3, 0.2), 13.4, 2.4, facecolor="#F3F3F3", edgecolor="#666", lw=1))
pseudo = (
    "FUNCTION Match(incoming):\n"
    "   remaining = incoming.amount\n"
    "   WHILE remaining > 0 AND opposite side not empty:\n"
    "      best = best price level (Asks[0] if BUY else Bids[0])\n"
    "      IF type == LIMIT AND best.price exceeds incoming.price -> BREAK\n"
    "      FOR EACH resting in best.orders  (FIFO - first come first serve):\n"
    "         matchQty = min(remaining, resting.remaining())\n"
    "         APPEND TradeResult(buyer, seller, price=best.price, amount=matchQty)\n"
    "         remaining -= matchQty;  update filledAmount on both sides\n"
    "         IF resting fully filled -> remove from price level\n"
    "      IF price level empty -> pop level\n"
    "   RETURN trades"
)
ax.text(0.5, 2.4, pseudo, fontsize=8.5, family="monospace", va="top")

plt.title("Hình 10. Cấu trúc Order Book và thuật toán khớp lệnh Price-Time Priority",
          fontsize=12, fontweight="bold", pad=12, color=C_ACCENT)
plt.savefig(os.path.join(OUT, "fig10-orderbook.png"),
            dpi=170, bbox_inches="tight", facecolor=C_BG)
plt.close()
print("OK fig10")

print("DONE all diagrams")
