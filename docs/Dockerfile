# The site builds once, on the build host, and the nginx stage is per target platform.
FROM --platform=$BUILDPLATFORM node:24-alpine@sha256:06d7adf3e0ef61a13f1d28682726366f97a15aa1888c934d3ecec86c33918b53 AS build
WORKDIR /site
COPY package.json package-lock.json ./
RUN npm ci
COPY . .
RUN npm run build

# The stock configuration serves /usr/share/nginx/html on 8080; every page is a <path>/index.html (trailingSlash).
FROM nginxinc/nginx-unprivileged:1.29-alpine@sha256:0c79d56aee561a1d81c63f00eee5fb5fe29279560cdc55e91425133104c7fbe6
COPY --from=build /site/build /usr/share/nginx/html
EXPOSE 8080
