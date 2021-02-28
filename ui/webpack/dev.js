/**
 * Created by: Andrey Polyakov (andrey@polyakov.im)
 */
import ReactRefreshWebpackPlugin from '@pmmmwh/react-refresh-webpack-plugin';

import {devServerConfig} from './config';
import {HotModuleReplacementPlugin} from "webpack";

export default {
    devtool: 'cheap-module-source-map',
    plugins: [new HotModuleReplacementPlugin(), new ReactRefreshWebpackPlugin()],
    devServer: devServerConfig,
};
