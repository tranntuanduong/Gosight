# GoSight: Real-time User Analytics & Session Replay Platform

## 1. Tổng quan dự án (Project Overview)
**GoSight** là một nền tảng quan sát (Observability Platform) tập trung vào hành vi người dùng (User Behavior). Hệ thống được thiết kế để xử lý lượng dữ liệu lớn (High Throughput) với mục tiêu đạt mốc **10.000 events/giây**.

* **Tư duy cốt lõi:** Biến dữ liệu thô (clicks, scrolls) thành insights (Rage clicks, Dead clicks, Session Replay).
* **Công nghệ chủ đạo:** Golang, Kafka, ClickHouse, gRPC, Protobuf, rrweb.

---

## 2. Kiến trúc hệ thống (System Architecture)

Hệ thống được thiết kế theo mô hình **Event-Driven Pipeline**:

1.  **SDK Layer (Client):** Nhúng vào ứng dụng (Web/NestJS). Gom batch các sự kiện DOM và gửi về trung tâm.
2.  **Ingestion Layer (Golang):** gRPC Server hiệu năng cao, tiếp nhận và validate dữ liệu, sau đó đẩy vào Kafka.
3.  **Buffering Layer (Kafka):** Đóng vai trò là "giảm chấn", đảm bảo tính toàn vẹn dữ liệu và hỗ trợ mở rộng (Scaling).
4.  **Processing Layer (Go Consumer):** Đọc dữ liệu từ Kafka, thực hiện các logic nghiệp vụ (phát hiện Rage click, làm giàu dữ liệu địa lý).
5.  **Storage Layer (ClickHouse):** Lưu trữ dạng cột (Columnar Storage), tối ưu cho truy vấn phân tích (OLAP).
6.  **Visualization (Grafana/UI):** Hiển thị các chỉ số đo đạc và giao diện Replay video code.



---

## 3. Đặc tả nghiệp vụ (Business Requirements)

### 3.1. Theo dõi sự kiện (Event Tracking)
* **Mouse Events:** Click, Double click, Right click.
* **Navigation:** URL change, Page load, Page exit.
* **Interaction:** Scroll depth (%), Input change (masked for privacy).

### 3.2. Phân tích trải nghiệm (UX Insights)
* **Dead Click:** Người dùng click vào phần tử không có tính tương tác.
* **Rage Click:** Người dùng click > 5 lần/2 giây trên cùng một vùng tọa độ.
* **Thrashed Cursor:** Người dùng di chuyển chuột hỗn loạn (biểu hiện của sự bối rối).

### 3.3. Tái hiện phiên (Session Replay)
* Sử dụng kỹ thuật **DOM Mutation Tracking** (không quay video màn hình).
* Lưu trữ: `Full Snapshot` (HTML ban đầu) và `Incremental Diffs` (các thay đổi sau đó).

---

## 4. Thiết kế dữ liệu (Data Design)

### 4.1. Protobuf Schema (`telemetry.proto`)
Sử dụng dữ liệu nhị phân để tối ưu băng thông (giảm ~70% so với JSON).

```protobuf
syntax = "proto3";

package gosight;

message Event {
  string session_id = 1;
  string user_id = 2;
  string event_type = 3; // click, scroll, mutation...
  string payload = 4;    // JSON string chứa chi tiết sự kiện hoặc DOM mutation
  int64 timestamp = 5;
  map<string, string> metadata = 6; // browser, os, ip...
}

service Ingestor {
  rpc SendEvents(stream Event) returns (Status);
}