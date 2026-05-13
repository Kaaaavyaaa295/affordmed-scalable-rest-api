"use client";

import { useEffect, useState } from "react";
import { productsApi, Product } from "@/lib/api";

// In a real app, get token from cookie/context (e.g. next-auth session)
const getToken = () => typeof window !== "undefined" ? localStorage.getItem("token") || "" : "";

export default function ProductsPage() {
  const [products, setProducts] = useState<Product[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [search, setSearch] = useState("");
  const [category, setCategory] = useState("");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    const load = async () => {
      setLoading(true);
      setError("");
      try {
        const resp = await productsApi.list(
          { page, limit: 12, search: search || undefined, category: category || undefined },
          getToken()
        );
        setProducts(resp.data || []);
        setTotal(resp.total);
      } catch (e) {
        setError((e as Error).message);
      } finally {
        setLoading(false);
      }
    };
    const debounce = setTimeout(load, 300);
    return () => clearTimeout(debounce);
  }, [page, search, category]);

  const totalPages = Math.ceil(total / 12);

  return (
    <main className="container mx-auto px-4 py-8">
      <h1 className="text-2xl font-medium mb-6">Products</h1>

      {/* Search & Filter */}
      <div className="flex gap-3 mb-6">
        <input
          type="text"
          placeholder="Search products..."
          value={search}
          onChange={(e) => { setSearch(e.target.value); setPage(1); }}
          className="border rounded px-3 py-2 flex-1 text-sm"
        />
        <select
          value={category}
          onChange={(e) => { setCategory(e.target.value); setPage(1); }}
          className="border rounded px-3 py-2 text-sm"
        >
          <option value="">All categories</option>
          <option value="devices">Devices</option>
          <option value="diagnostics">Diagnostics</option>
          <option value="consumables">Consumables</option>
        </select>
      </div>

      {error && <div className="text-red-600 text-sm mb-4">{error}</div>}

      {loading ? (
        <div className="grid grid-cols-3 gap-4">
          {Array.from({ length: 6 }).map((_, i) => (
            <div key={i} className="h-48 bg-gray-100 rounded animate-pulse" />
          ))}
        </div>
      ) : (
        <>
          <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4 mb-6">
            {products.map((p) => (
              <ProductCard key={p.id} product={p} />
            ))}
          </div>

          {/* Pagination */}
          <div className="flex items-center gap-2 justify-center">
            <button
              onClick={() => setPage((v) => Math.max(1, v - 1))}
              disabled={page === 1}
              className="px-3 py-1.5 text-sm border rounded disabled:opacity-40"
            >
              Previous
            </button>
            <span className="text-sm text-gray-500">
              Page {page} of {totalPages} ({total} products)
            </span>
            <button
              onClick={() => setPage((v) => Math.min(totalPages, v + 1))}
              disabled={page >= totalPages}
              className="px-3 py-1.5 text-sm border rounded disabled:opacity-40"
            >
              Next
            </button>
          </div>
        </>
      )}
    </main>
  );
}

function ProductCard({ product }: { product: Product }) {
  return (
    <div className="border rounded-lg p-4 hover:shadow-sm transition-shadow">
      {product.image_url && (
        <img src={product.image_url} alt={product.name} className="w-full h-36 object-cover rounded mb-3" />
      )}
      <h3 className="font-medium text-sm mb-1 line-clamp-1">{product.name}</h3>
      <p className="text-xs text-gray-500 mb-2 line-clamp-2">{product.description}</p>
      <div className="flex items-center justify-between">
        <span className="font-semibold text-sm">₹{product.price.toLocaleString()}</span>
        <span className="text-xs bg-gray-100 px-2 py-0.5 rounded">{product.category}</span>
      </div>
    </div>
  );
}
