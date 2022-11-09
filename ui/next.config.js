/** @type {import('next').NextConfig} */
const nextConfig = {
	reactStrictMode: true,
	swcMinify: true,
	async rewrites() {
		return [
			{
				source: "/api/:slug*",
				destination: "http://127.0.0.1:4444/api/:slug*",
			},
		];
	},
};

module.exports = nextConfig;
