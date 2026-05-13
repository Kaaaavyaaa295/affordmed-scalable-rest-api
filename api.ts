// frontend/src/lib/api.ts
// Typed API client for the Go backend

const BASE_URL = process.env.NEXT_PUBLIC_API_URL || "http://localhost:8080/api/v1";

type RequestOptions = {
  method?: string;
  body?: unknown;
  token?: string;
};

async function request<T>(path: string, opts: RequestOptions = {}): Promise<T> {
  const headers: Record<string, string> = { "Content-Type": "application/json" };
  if (opts.token) headers["Authorization"] = `Bearer ${opts.token}`;

  const res = await fetch(`${BASE_URL}${path}`, {
    method: opts.method || "GET",
    headers,
    body: opts.body ? JSON.stringify(opts.body) : undefined,
  });

  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: res.statusText }));
    throw new Error(err.error || "API error");
  }
  return res.json();
}

// ─── Types ────────────────────────────────────────────────────────────────────

export interface User {
  id: string; email: string; role: string; created_at: string;
}

export interface AuthResponse {
  token: string; refresh_token: string; expires_in: number; user: User;
}

export interface Product {
  id: string; name: string; description: string; price: number;
  category: string; image_url: string; is_active: boolean; created_at: string;
}

export interface PaginatedProducts {
  data: Product[]; total: number; page: number; limit: number;
}

export interface OrderItem {
  id: string; product_id: string; quantity: number; unit_price: number;
  product?: Product;
}

export interface Order {
  id: string; user_id: string; status: string; total_amount: number;
  items?: OrderItem[]; created_at: string;
}

// ─── Auth API ─────────────────────────────────────────────────────────────────

export const authApi = {
  login: (email: string, password: string) =>
    request<AuthResponse>("/auth/login", { method: "POST", body: { email, password } }),

  register: (email: string, password: string) =>
    request<AuthResponse>("/auth/register", { method: "POST", body: { email, password } }),

  refresh: (refreshToken: string) =>
    request<AuthResponse>("/auth/refresh", { method: "POST", body: { refresh_token: refreshToken } }),
};

// ─── Products API ─────────────────────────────────────────────────────────────

export const productsApi = {
  list: (params: { page?: number; limit?: number; category?: string; search?: string }, token: string) => {
    const q = new URLSearchParams(Object.entries(params).filter(([, v]) => v != null) as [string, string][]);
    return request<PaginatedProducts>(`/products?${q}`, { token });
  },
  getById: (id: string, token: string) => request<Product>(`/products/${id}`, { token }),
  create: (data: Partial<Product> & { initial_qty: number }, token: string) =>
    request<Product>("/products", { method: "POST", body: data, token }),
  update: (id: string, data: Partial<Product>, token: string) =>
    request<Product>(`/products/${id}`, { method: "PUT", body: data, token }),
  delete: (id: string, token: string) =>
    request<{ message: string }>(`/products/${id}`, { method: "DELETE", token }),
};

// ─── Orders API ───────────────────────────────────────────────────────────────

export const ordersApi = {
  create: (items: { product_id: string; quantity: number }[], token: string) =>
    request<Order>("/orders", { method: "POST", body: { items }, token }),
  getById: (id: string, token: string) => request<Order>(`/orders/${id}`, { token }),
  list: (token: string, page = 1) => request<{ data: Order[]; total: number }>(`/orders?page=${page}`, { token }),
};

// ─── Inventory API ────────────────────────────────────────────────────────────

export const inventoryApi = {
  update: (productId: string, quantity: number, reason: string, token: string) =>
    request(`/inventory/${productId}`, { method: "PUT", body: { quantity, reason }, token }),
};

// ─── Health API ───────────────────────────────────────────────────────────────

export const healthApi = {
  check: () => request<{ status: string; services: Record<string, string>; uptime_s: number }>("/health"),
};
