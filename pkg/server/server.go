package server

import (
	"context"
	"errors"
	"github.com/oklog/run"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sirupsen/logrus"
	"net/http"
	"os"
	"os/signal"
	"probermesh/config"
	"probermesh/pkg/util"
	"syscall"
)

type ProberMeshServerOption struct {
	TargetsConfigPath, ICMPDiscoveryType, HTTPListenAddr, RPCListenAddr, AggregationInterval string
}

func BuildServerMode(so *ProberMeshServerOption) {
	verify := func() error {
		if discoveryType(so.ICMPDiscoveryType) == StaticDiscovery && len(so.TargetsConfigPath) == 0 {
			// static 模式下配置文件必须指定，否则icmp没数据，http也没数据；无意义
			return errors.New("flag -server.probe.file must be set when -server.icmp.discovery flag is 'static'")
		}

		// 动态模式下配置参数可以不指定，有icmp保底
		if len(so.TargetsConfigPath) > 0 {
			if err := config.InitConfig(so.TargetsConfigPath); err != nil {
				logrus.Errorln("server parse config failed ", err)
				return err
			}
		}

		return nil
	}
	if err := verify(); err != nil {
		logrus.Fatalln("server config verify failed ", err)
	}

	ctxAll, cancelAll := context.WithCancel(context.Background())

	// 解析agg interval
	aggD, err := util.ParseDuration(so.AggregationInterval)
	if err != nil {
		logrus.Errorln("agg interval parse failed ", err)
		return
	}

	// rpc server
	if err := startRpcServer(so.RPCListenAddr); err != nil {
		logrus.Fatalln("start rpc server failed ", err)
		return
	}

	var g run.Group

	// 首次上报后 再updatePool,否则update不到数据
	ready := make(chan struct{})
	{
		// 初始化targetsPool
		g.Add(func() error {
			newTargetsPool(ctxAll, config.Get(), ready, so.ICMPDiscoveryType).start()
			return nil
		}, func(err error) {
			cancelAll()
		})
	}

	{
		// health check 打点
		g.Add(func() error {
			newHealthDot(ctxAll, aggD, ready).dot()
			return nil
		}, func(e error) {
			cancelAll()
		})
	}

	{
		// aggregation
		g.Add(func() error {
			newAggregator(ctxAll, aggD).startAggregation()
			return nil
		}, func(err error) {
			cancelAll()
		})
	}

	{
		// http server
		mux := http.NewServeMux()
		mux.Handle("/metrics", promhttp.Handler())
		svc := http.Server{
			Addr:    so.HTTPListenAddr,
			Handler: mux,
		}

		errCh := make(chan error)
		go func() {
			errCh <- svc.ListenAndServe()
		}()

		g.Add(func() error {
			select {
			case <-errCh:
			case <-ctxAll.Done():
			}
			return svc.Shutdown(context.TODO())
		}, func(err error) {
			cancelAll()
		})
	}

	{
		// 信号管理
		term := make(chan os.Signal, 1)
		signal.Notify(term, os.Interrupt, syscall.SIGTERM)
		cancel := make(chan struct{})

		g.Add(func() error {
			select {
			case <-term:
				logrus.Warnln("优雅关闭ing...")
				cancelAll()
				return nil
			case <-cancel:
				return nil
			}
		}, func(err error) {
			close(cancel)
			logrus.Warnln("signal controller over")
		})
	}

	g.Run()
}
