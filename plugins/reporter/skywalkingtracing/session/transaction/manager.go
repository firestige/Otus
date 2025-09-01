package transaction

import (
	"sync"
	"time"

	"firestige.xyz/otus/internal/log"
	"firestige.xyz/otus/plugins/reporter/skywalkingtracing/types"
	"firestige.xyz/otus/plugins/reporter/skywalkingtracing/utils"
)

type PendingResponseContainer struct {
	responses    []types.SipResponse
	lastUpdateAt time.Time
}

type TransactionManager struct {
	store     *sync.Map // 使用事务ID作为标识
	listeners []types.TransactionListener
	buffer    *sync.Map
}

func NewTransactionManager() *TransactionManager {
	return &TransactionManager{
		store:     &sync.Map{},
		listeners: make([]types.TransactionListener, 0),
		buffer:    &sync.Map{},
	}
}

func (m *TransactionManager) RegisterListener(listener types.TransactionListener) {
	m.listeners = append(m.listeners, listener)
}

func (m *TransactionManager) CreateTransaction(msg types.SipMessage) *TransactionContext {
	if req, ok := msg.(types.SipRequest); ok {
		ua := utils.ParseUAType(req)
		var state TransactionState
		switch req.Method() {
		case types.MethodInvite:
			// 对于Invite请求，客户端和服务器端的处理逻辑不同
			if ua == types.UAClient {
				state = &InviteCallingState{}
			}
			if ua == types.UAServer {
				state = &InviteProceedingState{}
			}
		default:
			state = &NonInviteTryingState{}
		}
		if state == nil {
			log.GetLogger().WithField("Call-id", req.CallID()).WithField("ua", ua).Errorf("Unsupported request msg: %s", req.StartLine())
			return nil
		}
		tx := NewTransaction(req, state)
		m.store.Store(tx.ID(), tx)
		for _, listener := range m.listeners {
			listener.OnTransactionCreated(tx)
		}
		return tx
	}
	return nil
}

func (m *TransactionManager) HandleMessage(msg types.SipMessage) error {
	tx, exist := m.GetTransactionBySipMessage(msg)
	if !exist {
		if msg.IsRequest() {
			if req, ok := msg.(types.SipRequest); ok {
				tx = m.CreateTransaction(req)
				if tx != nil {
					log.GetLogger().Infof("Created new transaction: %s", tx.ID())
				}
			}
		} else {
			// 有可能请求还没到，先缓存响应
			txID := utils.BuildTransactionID(msg)
			resp := msg.(types.SipResponse)
			container, exists := m.buffer.Load(txID)
			if !exists {
				container = &PendingResponseContainer{
					responses:    make([]types.SipResponse, 0),
					lastUpdateAt: time.Now(),
				}
				m.buffer.Store(txID, container)
			}
			container.(*PendingResponseContainer).responses = append(container.(*PendingResponseContainer).responses, resp)
			container.(*PendingResponseContainer).lastUpdateAt = time.Now()
			return nil // 直接返回，等待下次请求到达
		}
	}
	if tx == nil {
		log.GetLogger().Warnf("No transaction found for message: %s", msg.StartLine())
		return nil
	}
	err := tx.HandleMessage(msg)
	if err == nil {
		if tx.state.IsTerminated() {
			for _, listener := range m.listeners {
				listener.OnTransactionTerminated(tx)
			}
		}
	}
	// 处理完请求查看是否有没处理的响应
	txID := tx.ID()
	container, exists := m.buffer.Load(txID)
	if exists {
		for _, resp := range container.(*PendingResponseContainer).responses {
			tx.HandleMessage(resp)
		}
		m.buffer.Delete(txID)
	}
	return err
}

func (m *TransactionManager) GetTransactionByID(id string) (*TransactionContext, bool) {
	ctx, exists := m.store.Load(id)
	if ctx == nil {
		return nil, exists
	} else {
		return ctx.(*TransactionContext), exists
	}
}

func (m *TransactionManager) GetTransactionBySipMessage(msg types.SipMessage) (*TransactionContext, bool) {
	txID := utils.BuildTransactionID(msg)
	ctx, exists := m.store.Load(txID)
	if ctx == nil {
		return nil, exists
	} else {
		return ctx.(*TransactionContext), exists
	}
}

func (m *TransactionManager) GetAllTransactions() []*TransactionContext {
	var allTransactions []*TransactionContext
	m.store.Range(func(key, value interface{}) bool {
		allTransactions = append(allTransactions, value.(*TransactionContext))
		return true
	})
	return allTransactions
}

func (m *TransactionManager) Clear() {
	ticker := time.NewTicker(5 * time.Second)
	go func() {
		defer ticker.Stop()
		for range ticker.C {
			m.clearExpiredPairs()
		}
	}()
}

func (m *TransactionManager) clearExpiredPairs() {
	now := time.Now()
	expiredThreshold := 30 * time.Second

	m.buffer.Range(func(key, value interface{}) bool {
		if now.Sub(value.(*PendingResponseContainer).lastUpdateAt) > expiredThreshold {
			m.buffer.Delete(key)
			log.GetLogger().WithField("Transaction-ID", key).
				Infof("Cleared expired transaction pair, last updated: %s", value.(*PendingResponseContainer).lastUpdateAt.Format(time.RFC3339))
		}
		return true
	})
}
