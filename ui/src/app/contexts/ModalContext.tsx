import React, {createContext, FC, ReactNode, useReducer} from 'react'

export enum ModalContextActions {
    SetContent = "SET_CONTENT"
}

type Actions =
    | {type: ModalContextActions.SetContent, payload: ReactNode}
type State = {
    content: ReactNode
};
type Dispatch = (action: Actions) => void
type ModalContext = {
    state: State;
    dispatch: Dispatch;
}

// initial context state
const initModalState: State = {
    content: undefined
}

const ModalContext = createContext<ModalContext | undefined>(undefined)

const modalReducer = (state: State, action: Actions): State => {
    switch (action.type) {
        case ModalContextActions.SetContent: {
            return {...state, content: action.payload}
        }
        default: {
            // @ts-ignore
            throw new Error(`Unhandled action type: ${action.type}`)
        }
    }
}

const ModalContextProvider: FC = (props): JSX.Element => {
    const [state, dispatch] = useReducer(modalReducer, initModalState)
    const value = {state, dispatch}

    return <ModalContext.Provider value={value}>{props.children}</ModalContext.Provider>
}

const useModalContext = (): [State, Dispatch] => {
    const context = React.useContext(ModalContext)

    if (context === undefined) {
        throw new Error('useModalContext must be used within a ModalContextProvider')
    }

    return [context.state, context.dispatch]
}

export {ModalContextProvider, useModalContext}