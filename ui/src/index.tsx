import React, {FC} from "react";
import ReactDOM from "react-dom";
import { hot } from 'react-hot-loader/root';

import "@assets/styles/scss/main.scss";
import {RootView} from "@app/views/RootView";

const EntryPointContent: FC = () => {
	return <RootView />
}

const EntryPoint: FC = hot(EntryPointContent);

ReactDOM.render(<EntryPoint />, document.querySelector("#root"))

