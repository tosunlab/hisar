# The site builds once, on the build host, and the nginx stage is per target platform.
FROM --platform=$BUILDPLATFORM node:24-alpine AS build
WORKDIR /site
COPY package.json package-lock.json ./
RUN npm ci
COPY . .
RUN npm run build

# The stock configuration serves /usr/share/nginx/html on 8080; every page is a <path>/index.html (trailingSlash).
FROM nginxinc/nginx-unprivileged:1.29-alpine
COPY --from=build /site/build /usr/share/nginx/html
EXPOSE 8080
