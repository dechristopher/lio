import isWindows from 'is-windows';
import {join} from "path";

import {rootDir} from "../utils/env";

const defaultPort = 8080;

const devServerHost = isWindows() ? '127.0.0.1' : 'localhost';

export const devServerUrl = `http://${devServerHost}:${defaultPort}`;

export const devServerConfig = {
    hot: true,
    open: true,
    inline: true,
    // overlay: false,
    // publicPath: '/',
    port: defaultPort,
    host: devServerHost,
    historyApiFallback: true,
    contentBasePublicPath: "/",
    // proxy: devServerProxyConfig,
    contentBase: join(rootDir, "./"),
    // headers: {'Access-Control-Allow-Origin': '*'},
    disableHostCheck: true,
    transportMode: "ws",
    compress: true
};